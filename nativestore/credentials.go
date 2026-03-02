package nativestore

import (
	"fmt"

	"github.com/docker/docker-credential-helpers/credentials"
	"github.com/spf13/viper"
)

const DefaultLabel string = "alfresco"
const UrlLabel string = DefaultLabel + ".url"
const ProtocolLabel string = DefaultLabel + ".protocol"
const InsecureLabel string = DefaultLabel + ".insecure"
const MaxItemsLabel string = DefaultLabel + ".maxItems"

func Set(url, username, secret string) error {
	creds := credentials.Credentials{
		ServerURL: url,
		Username:  username,
		Secret:    secret,
	}
	credentials.SetCredsLabel(DefaultLabel)
	return store.Add(&creds)
}

func Get(url string) (string, string, error) {
	credentials.SetCredsLabel(DefaultLabel)
	return store.Get(url)
}

func Delete(url string) error {
	credentials.SetCredsLabel(DefaultLabel)
	return store.Delete(url)
}

func GetConnectionDetails() (string, bool, bool, error) {
	var storedServer = viper.GetString(UrlLabel)
	if storedServer == "" {
		return "", false, false, fmt.Errorf("use 'alfresco config set' to provide connection details")
	}
	var protocol = viper.GetString(ProtocolLabel)
	var tls bool = false
	var insecure bool = false
	if protocol == "https" {
		tls = true
		insecure = viper.GetBool(InsecureLabel)
	}
	return storedServer, tls, insecure, nil
}
