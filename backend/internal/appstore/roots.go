package appstore

import (
	"crypto/x509"
	"encoding/base64"
	"strings"
)

// appleRootCAG3 is "Apple Root CA - G3" (DER, base64), the root of every
// StoreKit 2 / App Store Server signed payload. Downloaded from
// https://www.apple.com/certificateauthority/AppleRootCA-G3.cer
// SHA-256 63343ABFB89A6A03EBB57E9B3F5FA7BE7C4F5C756F3017B3A8C488C3653E9179
// (also in macOS SystemRootCertificates.keychain); valid until 2039-04-30.
const appleRootCAG3 = `
MIICQzCCAcmgAwIBAgIILcX8iNLFS5UwCgYIKoZIzj0EAwMwZzEbMBkGA1UEAwwSQXBwbGUgUm9v
dCBDQSAtIEczMSYwJAYDVQQLDB1BcHBsZSBDZXJ0aWZpY2F0aW9uIEF1dGhvcml0eTETMBEGA1UE
CgwKQXBwbGUgSW5jLjELMAkGA1UEBhMCVVMwHhcNMTQwNDMwMTgxOTA2WhcNMzkwNDMwMTgxOTA2
WjBnMRswGQYDVQQDDBJBcHBsZSBSb290IENBIC0gRzMxJjAkBgNVBAsMHUFwcGxlIENlcnRpZmlj
YXRpb24gQXV0aG9yaXR5MRMwEQYDVQQKDApBcHBsZSBJbmMuMQswCQYDVQQGEwJVUzB2MBAGByqG
SM49AgEGBSuBBAAiA2IABJjpLz1AcqTtkyJygRMc3RCV8cWjTnHcFBbZDuWmBSp3ZHtfTjjTuxxE
tX/1H7YyYl3J6YRbTzBPEVoA/VhYDKX1DyxNB0cTddqXl5dvMVztK517IDvYuVTZXpmkOlEKMaNC
MEAwHQYDVR0OBBYEFLuw3qFYM4iapIqZ3r6966/ayySrMA8GA1UdEwEB/wQFMAMBAf8wDgYDVR0P
AQH/BAQDAgEGMAoGCCqGSM49BAMDA2gAMGUCMQCD6cHEFl4aXTQY2e3v9GwOAEZLuN+yRhHFD/3m
eoyhpmvOwgPUnPWTxnS4at+qIxUCMG1mihDK1A3UT82NQz60imOlM27jbdoXt2QfyFMm+YhidDkL
F1vLUagM6BgD56KyKA==`

// AppleRootCA returns the pinned Apple Root CA - G3 certificate.
func AppleRootCA() *x509.Certificate {
	der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(appleRootCAG3), ""))
	if err != nil {
		panic("appstore: bad embedded root: " + err.Error())
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		panic("appstore: bad embedded root: " + err.Error())
	}
	return cert
}

// AppleRoots is a pool holding only the pinned Apple root (never the system
// pool: any other CA must not be able to mint "Apple" transactions).
func AppleRoots() *x509.CertPool {
	p := x509.NewCertPool()
	p.AddCert(AppleRootCA())
	return p
}
