package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"flag"
	"log"
	"net/http"
	"os"

	"golang.org/x/net/webdav"
)


type Auth struct {
        username string
        password string
}

func (auth *Auth) basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if ok {
			usernameHash := sha256.Sum256([]byte(username))
			passwordHash := sha256.Sum256([]byte(password))
			expectedUsernameHash := sha256.Sum256([]byte(auth.username))
			expectedPasswordHash := sha256.Sum256([]byte(auth.password))

			usernameMatch := (subtle.ConstantTimeCompare(usernameHash[:], expectedUsernameHash[:]) == 1)
			passwordMatch := (subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 1)

			if usernameMatch && passwordMatch {
				next.ServeHTTP(w, r)
				return
			}
		}

		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

func main() {
	bind := flag.String("bind", ":7777", "binding port")

	auth := &Auth{}
	auth.username = os.Getenv("AUTH_USERNAME")
	auth.password = os.Getenv("AUTH_PASSWORD")

	if auth.username == "" {
		log.Fatal("basic auth username must be provided")
	}

	if auth.password == "" {
		log.Fatal("basic auth password must be provided")
	}

	flag.Parse()
	var fs webdav.Dir = "."
	h := &webdav.Handler {
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
	}
	http.HandleFunc("/", auth.basicAuth(h.ServeHTTP))
	//then use the Handler.ServeHTTP Method as the http.HandleFunc
	log.Printf("runnin on port %s", *bind)
	err := http.ListenAndServe(*bind, nil)
	if err != nil {
		log.Print(err.Error())
	}
}
