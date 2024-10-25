package main

import (
	"context"
	"flag"
	"github.com/hneemann/quiz/data"
	"github.com/hneemann/quiz/server"
	"github.com/hneemann/quiz/server/session"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
)

func Authenticate(user, pass string) (string, bool, error) {
	return "helmut", true, nil
}

func main() {
	lectureFolder := flag.String("lectures", "/home/hneemann/Dokumente/DHBW/Projekte/Quiz/Beispiele", "lecture folder")
	dataFolder := flag.String("data", "sessionData", "data folder")
	cert := flag.String("cert", "", "certificate file e.g. cert.pem")
	key := flag.String("key", "", "key file e.g. key.pem")
	debug := flag.Bool("debug", true, "starts server in debug mode")
	port := flag.Int("port", 8080, "port")
	flag.Parse()

	lectures, err := data.ReadLectures(*lectureFolder)
	if err != nil {
		log.Fatal(err)
	}

	sessions := session.New(*dataFolder, lectures)

	mux := http.NewServeMux()
	mux.Handle("/assets/", Cache(http.FileServer(http.FS(server.Assets)), 60*8, *debug))
	mux.Handle("/", sessions.Wrap(server.CreateMain(lectures)))
	mux.Handle("/lecture/", sessions.Wrap(server.CreateLecture(lectures)))
	mux.Handle("/chapter/", sessions.Wrap(server.CreateChapter(lectures)))
	mux.Handle("/task/", sessions.Wrap(server.CreateTask(lectures)))
	mux.Handle("/admin/", sessions.WrapAdmin(server.CreateAdmin(lectures)))
	mux.Handle("/statistics/", sessions.WrapAdmin(server.CreateStatistics(lectures, sessions)))
	mux.Handle("/image/", Cache(server.CreateImages(lectures), 60, *debug))

	loginTemp := server.Templates.Lookup("login.html")
	mux.Handle("/login", session.LoginHandler(sessions, loginTemp, session.AuthFunc(Authenticate)))

	//myOidc.RegisterLogin(mux, "/login", "/auth/callback", []byte("test1234test1234"), sessions)

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
