// Package app содержит прямолинейные функции команд publish, doctor и status.
package app

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

// Credentials — загруженный ключ API RuStore.
type Credentials struct {
	KeyID string
	Key   *rsa.PrivateKey
}

// credentialsFile — собственный формат файла учётных данных CLI.
// RuStore выдаёт keyId и значение приватного ключа отдельно, поэтому
// это не заявленный экспорт магазина.
type credentialsFile struct {
	KeyID      string `json:"keyId"`
	PrivateKey string `json:"privateKey"`
}

const credentialsHint = `ожидается JSON {"keyId":"...","privateKey":"BASE64_DER_PKCS8_RSA"}`

// LoadCredentials читает и проверяет файл учётных данных.
func LoadCredentials(path string) (Credentials, error) {
	if path == "" {
		return Credentials{}, exitcode.WithHint(
			exitcode.Errorf(exitcode.KindInvalidCredentials, "не задан файл учётных данных"),
			"передайте --credentials или задайте RUSTORE_CREDENTIALS_FILE",
		)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidCredentials,
				fmt.Errorf("прочитать файл учётных данных %q: %w", path, err)),
			"проверьте путь в --credentials или RUSTORE_CREDENTIALS_FILE",
		)
	}

	var file credentialsFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return Credentials{}, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidCredentials,
				fmt.Errorf("разобрать файл учётных данных %q: %w", path, err)),
			credentialsHint,
		)
	}

	if file.KeyID == "" {
		return Credentials{}, exitcode.WithHint(
			exitcode.Errorf(exitcode.KindInvalidCredentials, "в файле учётных данных нет keyId"),
			credentialsHint,
		)
	}
	if file.PrivateKey == "" {
		return Credentials{}, exitcode.WithHint(
			exitcode.Errorf(exitcode.KindInvalidCredentials, "в файле учётных данных нет privateKey"),
			credentialsHint,
		)
	}

	der, err := base64.StdEncoding.DecodeString(file.PrivateKey)
	if err != nil {
		return Credentials{}, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidCredentials,
				errors.New("privateKey не является Base64")),
			"вставьте значение приватного ключа из консоли RuStore без изменений",
		)
	}

	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return Credentials{}, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidCredentials,
				errors.New("privateKey не является DER PKCS#8")),
			"поддерживается только Base64 DER PKCS#8 с ключом RSA; PEM и PKCS#1 не принимаются",
		)
	}

	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return Credentials{}, exitcode.WithHint(
			exitcode.Errorf(exitcode.KindInvalidCredentials,
				"privateKey содержит %T, а не RSA", parsed),
			"поддерживается только ключ RSA",
		)
	}
	if err := key.Validate(); err != nil {
		return Credentials{}, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidCredentials, errors.New("ключ RSA повреждён")),
			"заново скопируйте ключ из консоли RuStore",
		)
	}

	return Credentials{KeyID: file.KeyID, Key: key}, nil
}
