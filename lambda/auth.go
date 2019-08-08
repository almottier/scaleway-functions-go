package lambda

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log"
	"net/http"
	"os"

	jwt "github.com/dgrijalva/jwt-go"
)

// ApplicationClaim represents the claims related to an application
// composed of either NamespaceID or ApplicationID of the linked JWT
type ApplicationClaim struct {
	NamespaceID   string `json:"namespace_id"`
	ApplicationID string `json:"application_id"`
}

// Claims represents a custom JWT claims with a list of applications
type Claims struct {
	ApplicationsClaims []ApplicationClaim `json:"application_claim"`
	jwt.StandardClaims
}

var (
	errorInvalidClaims      = errors.New("Invalid Claims")
	errorInvalidPublicKey   = errors.New("Invalid public key")
	errorEmptyRequestToken  = errors.New("Authentication token was not provided in the request")
	errorInvalidApplication = errors.New("Application ID was not provided")
	errorInvalidNamespace   = errors.New("Namespace ID was not provided")
)

// Authenticate incoming request based on multiple factors:
// - 1: Whether the function's privacy has been set to private, if public, just leave this middleware
// - 2: Get the public key injected in this function runtime (done automatically by Scaleway)
// - 3: Check whether a Token has been sent via a specific Headers reserved by Scaleway
// - 4: Parse the incoming JWT with the public key
// - 5: Check the "Application Claims" linked to the JWT
// - 6: Both FunctionID and NamespaceID are injected via environment variables by Scaleway
// ---  so we have to check the authenticity of the incoming token by comparing the claims
func authenticate(req *http.Request) (err error) {
	isPublicFunction := os.Getenv("SCW_PUBLIC")
	if isPublicFunction == "true" {
		return
	}

	// Check that request holds an authentication token
	requestToken := req.Header.Get("SCW_FUNCTIONS_TOKEN")
	if requestToken == "" {
		err = errorEmptyRequestToken
		return
	}

	// Retrieve Public Key used to parse JWT
	publicKey := os.Getenv("SCW_PUBLIC_KEY")
	if publicKey == "" {
		err = errorInvalidPublicKey
		return
	}

	block, _ := pem.Decode([]byte(publicKey))
	if block == nil {
		err = errorInvalidPublicKey
		return
	}

	parsedKey, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil || parsedKey == nil {
		// Print additional error
		log.Print(err)
		err = errorInvalidPublicKey
		return
	}

	// Parse JWT and retrieve claims
	claims := jwt.MapClaims{}

	_, err = jwt.ParseWithClaims(requestToken, claims, func(token *jwt.Token) (i interface{}, e error) {
		return parsedKey, nil
	})
	if err != nil {
		return
	}

	marshalledClaims, err := json.Marshal(claims["application_claim"])
	if err != nil {
		return
	}

	parsedClaims := []ApplicationClaim{}
	if err = json.Unmarshal(marshalledClaims, &parsedClaims); err != nil {
		return
	}

	if len(parsedClaims) == 0 {
		err = errorInvalidClaims
		return
	}
	applicationClaims := parsedClaims[0]

	applicationID := os.Getenv("SCW_APPLICATION_ID")
	namespaceID := os.Getenv("SCW_NAMESPACE_ID")
	if applicationID == "" {
		err = errorInvalidApplication
		return
	} else if namespaceID == "" {
		err = errorInvalidNamespace
		return
	}

	// Check that the token's claims match with the injected Application or Namespace ID (depending on the scope of the token)
	if applicationClaims.NamespaceID != namespaceID && applicationClaims.ApplicationID != applicationID {
		err = errorInvalidClaims
	}
	return
}
