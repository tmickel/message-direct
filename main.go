package main

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	_ "github.com/lib/pq"
)

func run() error {
	db, err := sql.Open("postgres", os.Getenv("PG_DSN"))
	if err != nil {
		return err
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/")

		if id == "" {
			handleNew(db, w, r)
			return
		}

		var row struct {
			Data                   string
			Iv                     string
			DownloadCountRemaining sql.NullInt64
			ExpirationTime         sql.NullTime
		}

		if err := db.QueryRow(`SELECT data, iv, download_count_remaining, expiration_time FROM pages WHERE id = $1;`, id).Scan(
			&row.Data, &row.Iv, &row.DownloadCountRemaining, &row.ExpirationTime,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "page not found", http.StatusNotFound)
				if _, err := db.Exec(
					`DELETE FROM pages WHERE id = $1`,
					id,
				); err != nil {
					log.Printf("failed to delete expired record: %v", err)
				}
				return
			} else {
				log.Print(err)
				http.Error(w, "failed to read", http.StatusInternalServerError)
				return
			}
		}

		if row.ExpirationTime.Valid {
			if time.Now().After(row.ExpirationTime.Time) {
				http.Error(w, "page not found", http.StatusNotFound)
				return
			}
		}

		if row.DownloadCountRemaining.Valid {
			if row.DownloadCountRemaining.Int64 <= 1 { // if == 1, allowed to view but deletable now.
				if _, err := db.Exec(
					`DELETE FROM pages WHERE id = $1`,
					id,
				); err != nil {
					log.Printf("failed to delete record with no more views allowed: %v", err)
				}
			}
			if row.DownloadCountRemaining.Int64 < 1 {
				http.Error(w, "page not found", http.StatusNotFound)
				return
			}
			if _, err := db.Exec(
				`UPDATE pages SET download_count_remaining = download_count_remaining - 1 WHERE id = $1`,
				id,
			); err != nil {
				http.Error(w, "failed to update download count", http.StatusInternalServerError)
				return
			}
		}

		tplTxt, err := ioutil.ReadFile("secretpage.html")
		if err != nil {
			http.Error(w, "failed to read template", http.StatusInternalServerError)
			return
		}
		tpl := template.Must(template.New("").Parse(string(tplTxt)))
		if err := tpl.Execute(w, row); err != nil {
			http.Error(w, "failed to execute template", http.StatusInternalServerError)
			log.Println(err)
			return
		}
	})

	http.ListenAndServe(":80", nil)
	return nil
}

func handleNew(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		http.ServeFile(w, r, "new.html")
	case "POST":
		if err := r.ParseForm(); err != nil {
			fmt.Fprintf(w, "ParseForm() err: %v", err)
			return
		}

		id := uuid.Must(uuid.NewRandom()).String()
		data := r.FormValue("data")
		iv := r.FormValue("iv")

		expirationTimeInsert := sql.NullTime{}
		if expiration := r.FormValue("expiration"); expiration != "" {
			expirationMins, err := strconv.Atoi(expiration)
			if err != nil {
				http.Error(w, "invalid expiration minutes", http.StatusBadRequest)
				return
			}
			expirationTimeInsert.Time = time.Now().Add(time.Duration(expirationMins) * time.Minute)
			expirationTimeInsert.Valid = true
		}

		countLimitInsert := sql.NullInt64{}
		if countLimit := r.FormValue("countLimit"); countLimit != "" {
			countLimitInt, err := strconv.Atoi(countLimit)
			if err != nil {
				http.Error(w, "invalid count limit", http.StatusBadRequest)
				return
			}
			countLimitInsert.Int64 = int64(countLimitInt)
			countLimitInsert.Valid = true
		}

		if _, err := db.Exec(
			`INSERT INTO pages (id, data, iv, download_count_remaining, expiration_time) VALUES ($1, $2, $3, $4, $5)`,
			id, data, iv, countLimitInsert, expirationTimeInsert,
		); err != nil {
			http.Error(w, "failed to insert", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, id)
	default:
		fmt.Fprintf(w, "Sorry, only GET and POST methods are supported.")
	}
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
