package httpclient

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aborroy/alfresco-cli/nativestore"
)

type Format string

const (
	Json    Format = "json"
	Content Format = "content"
	None    Format = "none"
)

const HttpClientId string = "[HTTP]"

type HttpExecution struct {
	Method             string
	Data               string
	Url                string
	Parameters         url.Values
	Format             Format
	ResponseBodyOutput io.Writer
}

type HTTPStatusError struct {
	StatusCode int
	StatusText string
	Method     string
	URL        string
	Body       string
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return "http request failed"
	}
	if e.Body == "" {
		return fmt.Sprintf("%s %s returned %d %s", e.Method, e.URL, e.StatusCode, e.StatusText)
	}
	return fmt.Sprintf("%s %s returned %d %s: %s", e.Method, e.URL, e.StatusCode, e.StatusText, e.Body)
}

var validHttpResponse = map[int]bool{
	http.StatusOK:        true,
	http.StatusCreated:   true,
	http.StatusNoContent: true,
}

var requestTimeout = 30 * time.Second
var maxRetries = 2
var retryWait = 500 * time.Millisecond

func Configure(timeout time.Duration, retries int, wait time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("http-timeout must be greater than zero")
	}
	if retries < 0 {
		return fmt.Errorf("http-retries must be zero or greater")
	}
	if wait <= 0 {
		return fmt.Errorf("http-retry-wait must be greater than zero")
	}
	requestTimeout = timeout
	maxRetries = retries
	retryWait = wait
	return nil
}

func GetUrlParams(params map[string]string) url.Values {
	var parameters = url.Values{}
	for key, value := range params {
		parameters.Add(key, value)
	}
	return parameters
}

func setBasicAuthHeader(request *http.Request, username, password string) {
	auth := username + ":" + password
	request.Header.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(auth)))
}

func createHttpClient(tlsEnabled bool, insecureAllowed bool) *http.Client {
	var tlsConfig *tls.Config
	if tlsEnabled && insecureAllowed {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}
	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: requestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &client
}

func checkStatusResponse(httpStatus int) bool {
	return validHttpResponse[httpStatus]
}

func isRetryableMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

func shouldRetry(method string, statusCode int, err error, attempt int) bool {
	if attempt >= maxRetries || !isRetryableMethod(method) {
		return false
	}
	if err != nil {
		return true
	}
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusRequestTimeout ||
		statusCode >= http.StatusInternalServerError
}

func doRequestWithRetry(
	method string,
	client *http.Client,
	requestFactory func() (*http.Request, error),
	responseHandler func(*http.Response) error,
) error {
	for attempt := 0; ; attempt++ {
		request, err := requestFactory()
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			if shouldRetry(method, 0, err, attempt) {
				time.Sleep(retryWait * time.Duration(attempt+1))
				continue
			}
			return err
		}

		err = responseHandler(response)
		if err == nil {
			return nil
		}

		var statusErr *HTTPStatusError
		if errors.As(err, &statusErr) && shouldRetry(method, statusErr.StatusCode, nil, attempt) {
			time.Sleep(retryWait * time.Duration(attempt+1))
			continue
		}
		return err
	}
}

func closeResponseBody(response *http.Response, requestUrl string) {
	if err := response.Body.Close(); err != nil {
		log.Println(requestUrl + " - Failed to close response body - " + err.Error())
	}
}

func buildStatusError(response *http.Response, method string, requestUrl string) error {
	const maxBodySize = 4096
	bodyBytes, _ := io.ReadAll(io.LimitReader(response.Body, maxBodySize))
	body := strings.TrimSpace(string(bodyBytes))
	if len(bodyBytes) == maxBodySize {
		body += "...(truncated)"
	}
	return &HTTPStatusError{
		StatusCode: response.StatusCode,
		StatusText: http.StatusText(response.StatusCode),
		Method:     method,
		URL:        requestUrl,
		Body:       body,
	}
}

func Execute(execution *HttpExecution, usernameOverride string, passwordOverride string) error {
	storedServer, tlsEnabled, insecureAllowed, err := nativestore.GetConnectionDetails()
	if err != nil {
		return err
	}
	var username, password string
	if usernameOverride != "" {
		username = usernameOverride
		password = passwordOverride
	} else {
		username, password, err = nativestore.Get(storedServer)
		if err != nil {
			return err
		}
	}

	urlStr := storedServer + execution.Url
	if execution.Parameters != nil {
		urlStr = urlStr + "?" + execution.Parameters.Encode()
	}

	client := createHttpClient(tlsEnabled, insecureAllowed)
	return doRequestWithRetry(
		execution.Method,
		client,
		func() (*http.Request, error) {
			var payload io.Reader
			if execution.Format == Json {
				payload = bytes.NewBufferString(execution.Data)
			}
			request, err := http.NewRequest(execution.Method, urlStr, payload)
			if err != nil {
				return nil, err
			}
			setBasicAuthHeader(request, username, password)
			if execution.Format == Json {
				request.Header.Set("Content-Type", "application/json; charset=UTF-8")
			}
			return request, nil
		},
		func(response *http.Response) error {
			defer closeResponseBody(response, urlStr)
			if !checkStatusResponse(response.StatusCode) {
				return buildStatusError(response, execution.Method, urlStr)
			}
			if execution.ResponseBodyOutput == nil {
				return nil
			}
			_, err := io.Copy(execution.ResponseBodyOutput, response.Body)
			return err
		},
	)
}

func ExecuteUploadContent(execution *HttpExecution, usernameOverride string, passwordOverride string) error {
	storedServer, tlsEnabled, insecureAllowed, err := nativestore.GetConnectionDetails()
	if err != nil {
		return err
	}
	var username, password string
	if usernameOverride != "" {
		username = usernameOverride
		password = passwordOverride
	} else {
		username, password, err = nativestore.Get(storedServer)
		if err != nil {
			return err
		}
	}

	r, w := io.Pipe()
	urlStr := storedServer + execution.Url
	request, err := http.NewRequest(execution.Method, urlStr, r)
	if err != nil {
		return err
	}
	setBasicAuthHeader(request, username, password)

	go func() {
		defer w.Close()
		file, err := os.Open(execution.Data)
		if err != nil {
			log.Println(err)
			return
		}
		defer file.Close()
		if _, err = io.Copy(w, file); err != nil {
			log.Println(err)
			return
		}
	}()

	response, err := createHttpClient(tlsEnabled, insecureAllowed).Do(request)
	if err != nil {
		return err
	}
	defer closeResponseBody(response, urlStr)
	if !checkStatusResponse(response.StatusCode) {
		return buildStatusError(response, execution.Method, urlStr)
	}
	if execution.ResponseBodyOutput == nil {
		return nil
	}
	_, err = io.Copy(execution.ResponseBodyOutput, response.Body)
	return err
}

func ExecuteDownloadContent(execution *HttpExecution, usernameOverride string, passwordOverride string) error {
	storedServer, tlsEnabled, insecureAllowed, err := nativestore.GetConnectionDetails()
	if err != nil {
		return err
	}
	var username, password string
	if usernameOverride != "" {
		username = usernameOverride
		password = passwordOverride
	} else {
		username, password, err = nativestore.Get(storedServer)
		if err != nil {
			return err
		}
	}

	urlStr := storedServer + execution.Url
	client := createHttpClient(tlsEnabled, insecureAllowed)
	return doRequestWithRetry(
		execution.Method,
		client,
		func() (*http.Request, error) {
			request, err := http.NewRequest(execution.Method, urlStr, nil)
			if err != nil {
				return nil, err
			}
			setBasicAuthHeader(request, username, password)
			return request, nil
		},
		func(response *http.Response) error {
			defer closeResponseBody(response, urlStr)
			if !checkStatusResponse(response.StatusCode) {
				return buildStatusError(response, execution.Method, urlStr)
			}
			out, err := os.Create(execution.Data)
			if err != nil {
				return err
			}
			defer out.Close()
			if _, err := io.Copy(out, response.Body); err != nil {
				_ = os.Remove(execution.Data)
				return err
			}
			if execution.ResponseBodyOutput != nil {
				_, _ = io.WriteString(execution.ResponseBodyOutput, execution.Data)
			}
			return nil
		},
	)
}
