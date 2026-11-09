package artifact

import (
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/vyachin/rustore-publisher/internal/artifact/artifacttest"
)

func TestParseManifest(t *testing.T) {
	t.Parallel()

	identity, err := parseManifest(artifacttest.Manifest("com.example.app", 42, "1.2.3"))
	if err != nil {
		t.Fatalf("parseManifest: %v", err)
	}
	if identity.Package != "com.example.app" || identity.VersionCode != 42 || identity.VersionName != "1.2.3" {
		t.Fatalf("identity = %+v", identity)
	}
}

func TestParseManifestWithoutVersionName(t *testing.T) {
	t.Parallel()

	identity, err := parseManifest(artifacttest.Manifest("com.example.app", 1, ""))
	if err != nil {
		t.Fatalf("parseManifest: %v", err)
	}
	if identity.Package != "com.example.app" || identity.VersionCode != 1 || identity.VersionName != "" {
		t.Fatalf("identity = %+v", identity)
	}
}

func TestParseManifestErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "пустые данные", data: nil, want: "корнев"},
		{name: "пустой package", data: artifacttest.Manifest("", 1, ""), want: "package пуст"},
		{name: "нулевой versionCode", data: artifacttest.Manifest("com.example.app", 0, ""), want: "не положителен"},
		{name: "мусор", data: []byte{0xff, 0xff}, want: ""},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseManifest(test.data)
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if test.want != "" && !strings.Contains(err.Error(), test.want) {
				t.Errorf("err = %q, want %q", err, test.want)
			}
		})
	}
}

func TestParseManifestRejectsVersionCodeMajor(t *testing.T) {
	t.Parallel()

	data := manifestWithMajor(42, 1)
	_, err := parseManifest(data)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if !strings.Contains(err.Error(), "versionCodeMajor") {
		t.Errorf("err = %q", err)
	}
}

// manifestWithMajor собирает манифест с android:versionCodeMajor.
func manifestWithMajor(versionCode, major int64) []byte {
	attribute := func(namespaceURI, name, value string) []byte {
		var out []byte
		if namespaceURI != "" {
			out = appendStringField(out, 1, namespaceURI)
		}
		out = appendStringField(out, 2, name)
		out = appendStringField(out, 3, value)
		return out
	}

	element := appendStringField(nil, 3, "manifest")
	element = appendBytesField(element, 4, attribute("", "package", "com.example.app"))
	element = appendBytesField(element, 4,
		attribute(androidNamespace, "versionCode", strconv.FormatInt(versionCode, 10)))
	element = appendBytesField(element, 4,
		attribute(androidNamespace, "versionCodeMajor", strconv.FormatInt(major, 10)))

	return appendBytesField(nil, 1, element)
}

func appendStringField(dst []byte, number protowire.Number, value string) []byte {
	dst = protowire.AppendTag(dst, number, protowire.BytesType)
	return protowire.AppendString(dst, value)
}

func appendBytesField(dst []byte, number protowire.Number, value []byte) []byte {
	dst = protowire.AppendTag(dst, number, protowire.BytesType)
	return protowire.AppendBytes(dst, value)
}
