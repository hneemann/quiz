package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/hneemann/quiz/data"
	"github.com/hneemann/quiz/server"
	"github.com/hneemann/quiz/server/myOidc"
	"github.com/hneemann/quiz/server/session"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

var errorTemp = server.Templates.Lookup("error.html")

// CatchPanic is a middleware that catches panics and displays them in a nicer way
func CatchPanic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				log.Println(r)
				err := errorTemp.Execute(writer, fmt.Sprint(r))
				if err != nil {
					log.Println(err)
				}
			}
		}()
		h.ServeHTTP(writer, request)
	})
}

func Authenticate(user, pass string) (string, bool, error) {
	if user == "admin" && pass == "admin" {
		return "admin", true, nil
	} else if user != "" {
		return user, false, nil
	}
	return "", false, errors.New("Kein Benutzername angegeben!")
}

func main() {
	dataFolder := flag.String("data", ".", "data folder")
	logFolder := flag.String("logs", "logs", "log folder")
	cert := flag.String("cert", "", "certificate file e.g. cert.pem")
	key := flag.String("key", "", "key file e.g. key.pem")
	cache := flag.Bool("cache", false, "enables browser caching for static content")
	port := flag.Int("port", 8080, "port")
	flag.Parse()

	var logPath string
	if strings.HasPrefix(*logFolder, "/") {
		logPath = *logFolder
	} else {
		logPath = filepath.Join(*dataFolder, *logFolder)
	}

	lectures, err := data.ReadLectures(ensureFolderExists(filepath.Join(*dataFolder, "lectures")))
	if err != nil {
		log.Fatal(err)
	}

	sessions := session.New(ensureFolderExists(filepath.Join(*dataFolder, "sessions")), lectures)

	states := data.NewLectureStates(filepath.Join(*dataFolder, "state"))

	mux := http.NewServeMux()

	isOidc := myOidc.RegisterLogin(mux, "/login", "/auth/callback",
		func(ident string, admin bool, w http.ResponseWriter) {
			sessions.Create(ident, admin, w)
		},
	)

	if !isOidc {
		log.Println("use simple dummy authenticator instead of oidc")
		loginTemp := server.Templates.Lookup("login.html")
		mux.Handle("/login", session.LoginHandler(sessions, loginTemp, session.AuthFunc(Authenticate)))
	}

	mux.Handle("/static/", Cache(http.FileServer(http.FS(server.Static)), 60*8, *cache))
	mux.Handle("/", sessions.Wrap(server.CreateMain(lectures, !isOidc, states)))
	mux.Handle("/lecture/", CatchPanic(sessions.Wrap(server.CreateLecture(lectures))))
	mux.Handle("/chapter/", CatchPanic(sessions.Wrap(server.CreateChapter(lectures, states))))
	mux.Handle("/task/", CatchPanic(sessions.Wrap(server.CreateTask(lectures, states))))
	mux.Handle("/admin/", CatchPanic(sessions.WrapAdmin(server.CreateAdmin(lectures))))
	mux.Handle("/statistics/", CatchPanic(sessions.WrapAdmin(server.CreateStatistics(lectures, sessions))))
	mux.Handle("/settings/", CatchPanic(sessions.WrapAdmin(server.CreateSettings(lectures, states))))
	mux.Handle("/logs/", CatchPanic(sessions.WrapAdmin(server.CreateLogs(logPath))))
	mux.Handle("/image/", CatchPanic(Cache(server.CreateImages(lectures), 60, *cache)))
	mux.Handle("/logout", session.LogoutHandler(sessions))

	serv := &http.Server{Addr: ":" + strconv.Itoa(*port), Handler: mux}

	c := make(chan os.Signal, 2)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-c
		log.Print("server received signal ", s)

		err := serv.Shutdown(context.Background())
		if err != nil {
			log.Println(err)
		}
		for {
			log.Print("server received signal ", <-c)
		}
	}()

	if *cert != "" && *key != "" {
		log.Println("start tls server at port", *port)
		err := serv.ListenAndServeTLS(*cert, *key)
		if err != nil {
			log.Println(err)
		}
	} else {
		log.Println("start server without tls at port", *port)
		err := serv.ListenAndServe()
		if err != nil {
			log.Println(err)
		}
	}

	sessions.PersistAll()
}

func ensureFolderExists(path string) string {
	fi, err := os.Stat(path)
	if err == nil {
		if !fi.IsDir() {
			log.Fatalf("path %s is not a directory", path)
		}
		return path
	}

	err = os.Mkdir(path, 0755)
	if err != nil {
		log.Fatal(err)
	}
	return path
}

func Cache(parent http.Handler, minutes int, enableCache bool) http.Handler {
	if enableCache {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Add("Cache-Control", "public, max-age="+strconv.Itoa(minutes*60))
			parent.ServeHTTP(writer, request)
		})
	} else {
		log.Println("browser caching disabled")
		return parent
	}
}
