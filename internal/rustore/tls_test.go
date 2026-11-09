package rustore

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestEmbeddedMinistryCertificates(t *testing.T) {
	read := func(name string) *x509.Certificate {
		t.Helper()
		data, err := embeddedCerts.ReadFile("certs/" + name)
		if err != nil {
			t.Fatalf("прочитать %s: %v", name, err)
		}
		block, _ := pem.Decode(data)
		if block == nil {
			t.Fatalf("%s не PEM", name)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatalf("разобрать %s: %v", name, err)
		}
		return cert
	}

	root := read("russian_trusted_root_ca.crt")
	sub := read("russian_trusted_sub_ca.crt")

	if !root.IsCA || root.Subject.CommonName != "Russian Trusted Root CA" {
		t.Errorf("root = %s, IsCA=%v", root.Subject, root.IsCA)
	}
	if !sub.IsCA || sub.Subject.CommonName != "Russian Trusted Sub CA" {
		t.Errorf("sub = %s, IsCA=%v", sub.Subject, sub.IsCA)
	}
	if err := sub.CheckSignatureFrom(root); err != nil {
		t.Errorf("промежуточный сертификат не подписан корнем: %v", err)
	}

	pool, err := rootCAs(false)
	if err != nil {
		t.Fatalf("rootCAs: %v", err)
	}
	if _, err := sub.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		t.Errorf("пул доверия не проверяет встроенную цепочку: %v", err)
	}
}

func TestHTTPClientRejectsRedirects(t *testing.T) {
	var targetHits int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		atomic.AddInt32(&targetHits, 1)
	}))
	t.Cleanup(target.Close)

	server := httptest.NewServer(http.RedirectHandler(target.URL, http.StatusFound))
	t.Cleanup(server.Close)

	client, err := newHTTPClient(false)
	if err != nil {
		t.Fatalf("newHTTPClient: %v", err)
	}
	response, err := client.Get(server.URL)
	if response != nil {
		_ = response.Body.Close()
	}
	if err == nil {
		t.Fatal("редирект не отклонён")
	}
	if hits := atomic.LoadInt32(&targetHits); hits != 0 {
		t.Errorf("запрос ушёл на другой хост %d раз", hits)
	}
}
