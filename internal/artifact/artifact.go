// Package artifact проверяет Android App Bundle и читает сведения из манифеста.
package artifact

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"

	"github.com/vyachin/rustore-publisher/internal/exitcode"
)

// Info описывает проверенный Android App Bundle.
type Info struct {
	Path   string
	Size   int64
	SHA256 string
	Signed bool
}

// Bundle — открытый AAB вместе с результатом проверки и сведениями
// о версии из базового манифеста. Вызывающий код владеет File до Close.
type Bundle struct {
	File     *os.File
	Info     Info
	Identity Identity
}

// Open открывает и проверяет AAB и разбирает базовый манифест. Файл
// возвращается с начальной позиции и закрывается через Bundle.Close.
func Open(path string) (*Bundle, error) {
	file, size, reader, err := openBundle(path)
	if err != nil {
		return nil, err
	}

	identity, err := readIdentity(reader, path)
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	info, err := inspectOpened(file, path, size, reader)
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	return &Bundle{File: file, Info: info, Identity: identity}, nil
}

// Close закрывает файл. Безопасно вызывать для нулевого Bundle
// и несколько раз.
func (b *Bundle) Close() error {
	if b == nil || b.File == nil {
		return nil
	}
	file := b.File
	b.File = nil
	return file.Close()
}

// Rewind возвращает файл в начало перед загрузкой.
func (b *Bundle) Rewind() error {
	if b == nil || b.File == nil {
		return exitcode.Errorf(exitcode.KindInternal, "AAB не открыт")
	}
	_, err := b.File.Seek(0, io.SeekStart)
	return err
}

// Inspect проверяет, что path — читаемый Android App Bundle, и возвращает
// размер и SHA-256. Манифест не разбирается; для данных версии нужен Open.
func Inspect(path string) (Info, error) {
	file, size, reader, err := openBundle(path)
	if err != nil {
		return Info{}, err
	}
	defer func() { _ = file.Close() }()

	return inspectOpened(file, path, size, reader)
}

func openBundle(path string) (*os.File, int64, *zip.Reader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, nil, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidAAB, err),
			"проверьте путь, переданный в --aab",
		)
	}

	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, nil, exitcode.Wrap(exitcode.KindInvalidAAB, err)
	}
	if stat.Size() == 0 {
		_ = file.Close()
		return nil, 0, nil, exitcode.Errorf(exitcode.KindInvalidAAB, "%s пуст", path)
	}

	reader, err := zip.NewReader(file, stat.Size())
	if err != nil {
		_ = file.Close()
		return nil, 0, nil, exitcode.WithHint(
			exitcode.Wrap(exitcode.KindInvalidAAB, err),
			"передайте подписанный .aab, собранный Gradle или bundletool",
		)
	}

	if !hasBundleConfig(reader) {
		_ = file.Close()
		return nil, 0, nil, exitcode.WithHint(
			exitcode.Errorf(exitcode.KindInvalidAAB, "%s не является Android App Bundle: не найден BundleConfig.pb", path),
			"APK не поддерживается; передайте файл .aab",
		)
	}

	return file, stat.Size(), reader, nil
}

func inspectOpened(file *os.File, path string, size int64, reader *zip.Reader) (Info, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return Info{}, exitcode.Wrap(exitcode.KindInvalidAAB, err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return Info{}, exitcode.Wrap(exitcode.KindInvalidAAB, err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return Info{}, exitcode.Wrap(exitcode.KindInvalidAAB, err)
	}

	return Info{
		Path:   path,
		Size:   size,
		SHA256: hex.EncodeToString(hash.Sum(nil)),
		Signed: hasSignature(reader),
	}, nil
}

// hasSignature сообщает, есть ли в AAB подпись JAR: так Gradle и bundletool
// подписывают наборы.
func hasSignature(reader *zip.Reader) bool {
	for _, file := range reader.File {
		name := strings.ToLower(file.Name)
		if !strings.HasPrefix(name, "meta-inf/") {
			continue
		}
		if strings.HasSuffix(name, ".rsa") || strings.HasSuffix(name, ".ec") || strings.HasSuffix(name, ".dsa") {
			return true
		}
	}
	return false
}

func hasBundleConfig(reader *zip.Reader) bool {
	for _, file := range reader.File {
		if file.Name == "BundleConfig.pb" {
			return true
		}
	}
	return false
}
