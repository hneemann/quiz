package main

import (
	"context"
	"errors"
	"flag"
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
)

func Authenticate(user, pass string) (string, bool, error) {
	if user == "admin" && pass == "admin" {
		return "admin", true, nil
	} else if user != "" {
		return user, false, nil
	}
	return "", false, errors.New("no user")
}

func main() {
	dataFolder := flag.String("data", "/home/hneemann/Dokumente/DHBW/Projekte/Quiz", "data folder")
	cert := flag.String("cert", "", "certificate file e.g. cert.pem")
	key := flag.String("key", "", "key file e.g. key.pem")
	debug := flag.Bool("debug", true, "starts server in debug mode")
	port := flag.Int("port", 8080, "port")
	flag.Parse()

	lectures, err := data.ReadLectures(ensureFolderExists(filepath.Join(*dataFolder, "lectures")))
	if err != nil {
		log.Fatal(err)
	}

	sessions := session.New(ensureFolderExists(filepath.Join(*dataFolder, "sessions")), lectures)

	states := data.NewLectureStates(filepath.Join(*dataFolder, "state"))

	mux := http.NewServeMux()
	mux.Handle("/assets/", Cache(http.FileServer(http.FS(server.Assets)), 60*8, *debug))
	mux.Handle("/", sessions.Wrap(server.CreateMain(lectures)))
	mux.Handle("/lecture/", sessions.Wrap(server.CreateLecture(lectures)))
	mux.Handle("/chapter/", sessions.Wrap(server.CreateChapter(lectures, states)))
	mux.Handle("/task/", sessions.Wrap(server.CreateTask(lectures, states)))
	mux.Handle("/admin/", sessions.WrapAdmin(server.CreateAdmin(lectures)))
	mux.Handle("/statistics/", sessions.WrapAdmin(server.CreateStatistics(lectures, sessions)))
	mux.Handle("/settings/", sessions.WrapAdmin(server.CreateSettings(lectures, states)))
	mux.Handle("/image/", Cache(server.CreateImages(lectures), 60, *debug))
	mux.Handle("/logout", sessions.Wrap(session.LogoutHandler(sessions)))

	isOidc := myOidc.RegisterLogin(mux, "/login", "/auth/callback", []byte("test1234test1234"), sessions)

	if !isOidc {
		if *debug {
			log.Println("start server without login")
			loginTemp := server.Templates.Lookup("login.html")
			mux.Handle("/login", session.LoginHandler(sessions, loginTemp, session.AuthFunc(Authenticate)))
		} else {
			log.Fatal("no login method")
		}
	}

	serv := &http.Server{Addr: ":" + strconv.Itoa(*port), Handler: mux}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		log.Print("terminated")

		err := serv.Shutdown(context.Background())
		if err != nil {
			log.Println(err)
		}
		for {
			<-c
		}
	}()

	if *cert != "" && *key != "" {
		log.Println("start tls server")
		err := serv.ListenAndServeTLS(*cert, *key)
		if err != nil {
			log.Println(err)
		}
	} else {
		err := serv.ListenAndServe()
		if err != nil {
			log.Println(err)
		}
	}

	sessions.PersistAll()
}

func ensureFolderExists(path string) string {
	err := os.MkdirAll(path, 0755)
	if err != nil {
		log.Fatal(err)
	}
	return path
}

func Cache(parent http.Handler, minutes int, debug bool) http.Handler {
	if debug {
		log.Println("Cache disabled")
		return parent
	} else {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Add("Cache-Control", "public, max-age="+strconv.Itoa(minutes*60))
			parent.ServeHTTP(writer, request)
		})
	}
}
