package myOidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

func createRandomBytes(size int) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = byte(rand.Int())
	}
	return b
}

type CreateSession func(ident string, admin bool, w http.ResponseWriter)

func RegisterLogin(mux *http.ServeMux, loginPath, callbackPath string, createSession CreateSession) bool {
	clientID := os.Getenv("CLIENT_ID")
	clientSecret := os.Getenv("CLIENT_SECRET")
	keyPath := os.Getenv("KEY_PATH")
	issuer := os.Getenv("ISSUER")
	tokenIdAttr := os.Getenv("ID_TOKEN_ID_ATTR")
	tokenRolesAttr := os.Getenv("ID_TOKEN_ROLES_ATTR")
	tokenRoleAdminValue := os.Getenv("ID_TOKEN_ADMIN_VALUE")
	callbackHost := os.Getenv("HOST")
	scopes := strings.Split(os.Getenv("SCOPES"), " ")
	responseMode := os.Getenv("RESPONSE_MODE")

	if clientID == "" || issuer == "" || callbackHost == "" {
		log.Println("missing oidc environment variables: CLIENT_ID, ISSUER, CLIENT_SECRET, HOST, SCOPES, ID_TOKEN_ID_ATTR, ID_TOKEN_ROLE_ATTR, ID_TOKEN_ADMIN_VALUE")
		return false
	}

	redirectURI := callbackHost + callbackPath
	cookieHandler := httphelper.NewCookieHandler(createRandomBytes(16), createRandomBytes(16), httphelper.WithUnsecure())

	client := &http.Client{
		Timeout: time.Minute,
	}
	// enable outgoing request logging

	options := []rp.Option{
		rp.WithCookieHandler(cookieHandler),
		rp.WithVerifierOpts(rp.WithIssuedAtOffset(5 * time.Second)),
		rp.WithHTTPClient(client),
		rp.WithSigningAlgsFromDiscovery(),
	}
	if clientSecret == "" {
		options = append(options, rp.WithPKCE(cookieHandler))
	}
	if keyPath != "" {
		options = append(options, rp.WithJWTProfile(rp.SignerFromKeyPath(keyPath)))
	}

	retry := 0
	var provider rp.RelyingParty
	for {
		var err error
		provider, err = rp.NewRelyingPartyOIDC(context.Background(), issuer, clientID, clientSecret, redirectURI, scopes, options...)
		if err != nil {
			if retry > 2 {
				log.Fatalf("error creating provider: %v, give up!", err.Error())
			} else {
				log.Printf("error creating provider: %v, retrying in 2 seconds", err.Error())
				time.Sleep(2 * time.Second)
				retry++
			}
		} else {
			if retry > 0 {
				log.Printf("provider created after %d retries", retry)
			}
			break
		}
	}

	// generate some state (representing the state of the user in your application,
	// e.g. the page where he was before sending him to login
	state := func() string {
		return uuid.New().String()
	}

	urlOptions := []rp.URLParamOpt{
		rp.WithPromptURLParam("Welcome back!"),
	}

	if responseMode != "" {
		urlOptions = append(urlOptions, rp.WithResponseModeURLParam(oidc.ResponseMode(responseMode)))
	}

	// register the AuthURLHandler at your preferred path.
	// the AuthURLHandler creates the auth request and redirects the user to the auth server.
	// including state handling with secure cookie and the possibility to use PKCE.
	// Prompts can optionally be set to inform the server of
	// any messages that need to be prompted back to the user.
	mux.Handle(loginPath, rp.AuthURLHandler(
		state,
		provider,
		urlOptions...,
	))

	unmarshalToken := func(w http.ResponseWriter, r *http.Request, tokens *oidc.Tokens[*oidc.IDTokenClaims], state string, rp rp.RelyingParty) {
		tok := strings.Split(tokens.IDToken, ".")
		if len(tok) < 2 {
			http.Error(w, "no IDToken received", 504)
			return
		}

		b, err := base64.StdEncoding.WithPadding(base64.NoPadding).DecodeString(tok[1])
		if err != nil {
			log.Println(err)
			http.Error(w, "error decoding IDToken", 504)
			return
		}
		m := map[string]any{}
		err = json.Unmarshal(b, &m)
		if err != nil {
			log.Println(string(b))
			log.Println(err)
			http.Error(w, "error unmarshalling IDToken", 504)
			return
		}

		ident, ok := m[tokenIdAttr]
		if !ok {
			log.Println(m)
			http.Error(w, "no id found in IDToken", 504)
			return
		}

		admin := false
		if a, ok := m[tokenRolesAttr]; ok {
			if rs, ok := a.([]any); ok {
				for _, r := range rs {
					if fmt.Sprint(r) == tokenRoleAdminValue {
						admin = true
					}
				}
			}
		}

		log.Println("oidc id:", ident, admin)

		createSession(fmt.Sprint(ident), admin, w)
		http.Redirect(w, r, "/", http.StatusFound)
	}

	mux.Handle(callbackPath, rp.CodeExchangeHandler(unmarshalToken, provider))

	return true
}
