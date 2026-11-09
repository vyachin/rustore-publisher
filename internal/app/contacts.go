package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
	"github.com/vyachin/rustore-publisher/internal/rustore"
)

// ContactsFileName — файл контактов разработчика в каталоге метаданных.
const ContactsFileName = "developer-contacts.json"

// ReadDeveloperContacts читает и проверяет контакты разработчика.
// Достаточно одного заполненного способа связи.
func ReadDeveloperContacts(path string) (rustore.DeveloperContacts, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return rustore.DeveloperContacts{}, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidArguments,
				fmt.Errorf("прочитать контакты разработчика %q: %w", path, err)),
			"проверьте путь в --contacts-file или файл metadata/"+ContactsFileName,
		)
	}

	var contacts rustore.DeveloperContacts
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&contacts); err != nil {
		return rustore.DeveloperContacts{}, contactsError(
			"разобрать %q: %v", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return rustore.DeveloperContacts{}, contactsError(
			"в файле %q больше одного JSON-объекта", path)
	}

	contacts.Email = strings.TrimSpace(contacts.Email)
	contacts.Website = strings.TrimSpace(contacts.Website)
	contacts.VKCommunity = strings.TrimSpace(contacts.VKCommunity)
	contacts.MaxMessenger = strings.TrimSpace(contacts.MaxMessenger)

	if (rustore.DeveloperContacts{}) == contacts {
		return rustore.DeveloperContacts{}, contactsError(
			"в файле %q нет ни одного способа связи", path)
	}
	if contacts.Email != "" && !validEmail(contacts.Email) {
		return rustore.DeveloperContacts{}, contactsError(
			"некорректный email %q", contacts.Email)
	}
	for name, value := range map[string]string{
		"website":      contacts.Website,
		"vkCommunity":  contacts.VKCommunity,
		"maxMessenger": contacts.MaxMessenger,
	} {
		if value != "" && !validContactURL(value) {
			return rustore.DeveloperContacts{}, contactsError(
				"некорректный %s %q: нужен http(s)-адрес", name, value)
		}
	}

	return contacts, nil
}

// validEmail проверяет простой формат почты, достаточный для API.
func validEmail(value string) bool {
	if strings.Count(value, "@") != 1 || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	at := strings.Index(value, "@")
	return at > 0 && at < len(value)-3 && strings.Contains(value[at+1:], ".")
}

// validContactURL требует абсолютный http(s)-адрес с хостом.
func validContactURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

// contactsError создаёт ошибку ввода с подсказкой о формате файла.
func contactsError(format string, args ...any) error {
	return exitcode.WithHint(
		exitcode.Errorf(exitcode.KindInvalidArguments, format, args...),
		`ожидается JSON {"email":"...","website":"https://...","vkCommunity":"...","maxMessenger":"..."} хотя бы с одним полем`,
	)
}
