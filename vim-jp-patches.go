package main

import (
	"crypto/sha1"
	"database/sql"
	"flag"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/hoisie/web"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"
)

const uri = "http://ftp.vim.org/vim/unstable/patches/7.4a/"

func defaultAddr() string {
	port := os.Getenv("PORT")
	if port == "" {
		return ":80"
	}
	return ":" + port
}

var addr = flag.String("addr", defaultAddr(), "server address")

func updatePatches(db *sql.DB) {
	doc, err := goquery.NewDocument(uri)
	if err != nil {
		return
	}
	lines := strings.Split(doc.Find("pre").Text(), "\n")
	s, e := -1, -1
	sp := regexp.MustCompile(`^\s+SIZE\s+NAME\s+FIXES$`)
	ep := regexp.MustCompile(`^\s+\d`)
	for n, line := range lines {
		if s == -1 && sp.MatchString(line) {
			s = n
		} else if s != -1 && e == -1 && !ep.MatchString(line) {
			e = n
			break
		}
	}
	lines = lines[s+1 : e]

	tp := regexp.MustCompile(`^\s+\d+\s+(\S+)\s+(.*)$`)

	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Commit()
	sql := "insert into patches(name, title, description, created_at) values(?, ?, ?, ?)"
	secret := os.Getenv("VIM_JP_PATCHES_SECRET")
	for _, line := range lines {
		parts := tp.FindAllStringSubmatch(line, 1)[0]
		_, err = tx.Exec(sql, parts[1], parts[2], "", time.Now())
		if err == nil {
			sha1h := sha1.New()
			fmt.Fprint(sha1h, "vim_jp"+secret)
			params := make(url.Values)
			params.Set("room", "vim")
			params.Set("bot", "vim_jp")
			params.Set("text", fmt.Sprintf("%s\n%s", parts[1], parts[2]))
			params.Set("bot_verifier", fmt.Sprintf("%x", sha1h.Sum(nil)))
			r, err := http.Get("http://lingr.com/api/room/say?" + params.Encode())
			if err == nil {
				r.Body.Close()
			}
		}
	}
}

type Item struct {
	Id          string
	Title       string
	Link        string
	Description string
	Created     time.Time
}

func main() {
	flag.Parse()

	var mutex sync.Mutex

	db, err := sql.Open("sqlite3", "./patches.db")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	go func() {
		for {
			time.Sleep(10 * time.Minute)
			mutex.Lock()
			updatePatches(db)
			mutex.Unlock()
		}
	}()

	t, err := template.ParseFiles(filepath.Join(filepath.Dir(os.Args[0]), "feed.rss"))
	if err != nil {
		log.Fatal(err)
	}

	web.Get("/pull", func(ctx *web.Context) string {
		mutex.Lock()
		defer mutex.Unlock()
		updatePatches(db)
		return "OK"
	})

	web.Get("/", func(ctx *web.Context) string {
		mutex.Lock()
		defer mutex.Unlock()

		sql := "select name, title, created_at from patches order by created_at desc limit 10"
		rows, err := db.Query(sql)
		if err != nil {
			ctx.Abort(http.StatusInternalServerError, err.Error())
			return ""
		}
		defer rows.Close()

		items := make([]Item, 0)
		for rows.Next() {
			var name, title string
			var created_at time.Time
			err = rows.Scan(&name, &title, &created_at)
			if err != nil {
				ctx.Abort(http.StatusInternalServerError, err.Error())
				return ""
			}
			items = append(items, Item{
				Id:          name,
				Title:       name,
				Link:        fmt.Sprintf("%s%s", uri, name),
				Description: title,
				Created:     created_at,
			})
		}
		ctx.SetHeader("Content-Type", "application/rss+xml", true)
		t.Execute(ctx.ResponseWriter, items)
		return ""
	})

	web.Run(*addr)
}
