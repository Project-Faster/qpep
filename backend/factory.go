package backend

import "github.com/Project-Faster/quicly-go"

func init() {
	quicly.Initialize(quicly.Options{
		Logger:          nil,
		CertificateFile: "testcert.pem",
		CertificateKey:  "testkey.pem",
	})
}
