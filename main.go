package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const location = "Asia/Tokyo"

func Env_load() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
}

func main() {
	Env_load()
	var (
		signingSecret string
	)
	var spreadsheetId = os.Getenv("SHEET_ID")
	const valueRange = "A:B"
	const valueInputOption = "USER_ENTERED"
	const insertDataOption = "INSERT_ROWS"

	flag.StringVar(&signingSecret, "secret", os.Getenv("SLACK_SIGNING_SECRET"), "Your Slack app's signing secret")
	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		verifier, err := slack.NewSecretsVerifier(r.Header, signingSecret)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		r.Body = ioutil.NopCloser(io.TeeReader(r.Body, &verifier))
		s, err := slack.SlashCommandParse(r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err = verifier.Ensure(); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		loc, err := time.LoadLocation(location)
		if err != nil {
			loc = time.FixedZone(location, 9*60*60)
		}
		ctx := context.Background()
		srv, err := sheets.NewService(ctx, option.WithCredentialsFile("credentials.json"))
		if err != nil {
			log.Fatalf("Unable to retrieve Sheets client: %v", err)
		}

		switch s.Command {
		case "/record":
			params := &slack.Msg{Text: s.Text}
			b, err := json.Marshal(params)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)

			t := time.Now().In(loc)
			var now = fmt.Sprintln(t.Format("2006-01-02 15:04:05"))

			rb := &sheets.ValueRange{
				MajorDimension: "ROWS",
				Values: [][]interface{}{
					{now, s.Text},
				},
			}
			resp, err := srv.Spreadsheets.Values.Append(spreadsheetId, valueRange, rb).
				ValueInputOption(valueInputOption).InsertDataOption(insertDataOption).Context(ctx).Do()
			if err != nil {
				log.Fatalf("Unable to retrieve data from sheet: %v", err)
				log.Fatalf("%v", resp)
			}
		case "/logs":
			valueRenderOption := "FORMATTED_VALUE"
			dateTimeRenderOption := "FORMATTED_STRING"

			resp, err := srv.Spreadsheets.Values.Get(spreadsheetId, valueRange).ValueRenderOption(valueRenderOption).
				DateTimeRenderOption(dateTimeRenderOption).Context(ctx).Do()
			if err != nil {
				log.Fatal(err)
			}
			var slackMsg strings.Builder
			for _, v := range resp.Values {
				slackMsg.WriteString(fmt.Sprintf("%s %s[C]\n", v[0], v[1]))
			}
			b, err := json.Marshal(slack.Msg{Text: slackMsg.String()})
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
	fmt.Println("[INFO] Server listening")
	http.ListenAndServe(":3000", nil)
}
