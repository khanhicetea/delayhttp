package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"sync/atomic"

	"github.com/projectdiscovery/retryablehttp-go"
	"github.com/urfave/cli/v2"
)

func workerLog(workerID int, msg string) {
	log.Println("Worker", workerID, msg)
}

func worker(workerID int, hc *http.Client, jobs <-chan []byte, done *uint64, failed *uint64) {
	workerLog(workerID, "Ready to work ...")

	for j := range jobs {
		workerLog(workerID, "Fetching")

		reader := bytes.NewReader(j)
		buf := bufio.NewReader(reader)
		req, _ := http.ReadRequest(buf)

		newHost := req.Header.Get("X-Delay-Host")
		req.Header.Del("X-Delay-Host")

		req.RequestURI = ""
		req.URL.Host = newHost
		req.Host = newHost

		if req.URL.Scheme == "" {
			req.URL.Scheme = "https"
		}

		_, err := hc.Do(req)

		if err != nil {
			fmt.Println(err)
			atomic.AddUint64(failed, 1)
			break
		}

		workerLog(workerID, "Fetched")
		atomic.AddUint64(done, 1)
	}
}

func serve(bind string, port string, worker_num uint64) {
	hc := retryablehttp.DefaultPooledClient()
	jobs := make(chan []byte, 1000)
	var received, done, failed uint64

	for i := 0; i < int(worker_num); i++ {
		go worker(i+1, hc, jobs, &done, &failed)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Skip
		if r.Header.Get("X-Delay-Host") == "" {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, "Not Found")
			return
		}

		serializedReq, _ := httputil.DumpRequest(r, true)
		jobs <- serializedReq
		atomic.AddUint64(&received, 1)
		io.WriteString(w, "OK")
	})

	mux.HandleFunc("/_stats", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, fmt.Sprintf("Received : [%d] = Done [%d] + Failed [%d] + Working [%d]", received, done, failed, (received-done-failed)))
	})

	err := http.ListenAndServe(fmt.Sprintf("%s:%s", bind, port), mux)

	if err != nil {
		fmt.Println("Error ! Exited !")
	}
}

func main() {
	app := &cli.App{
		Name:        "delayhttp",
		Description: "Delay HTTP Server",
		Commands: []*cli.Command{
			{
				Name:    "server",
				Aliases: []string{"s"},
				Usage:   "Run HTTP server",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "bind",
						Usage: "Binding address",
						Value: "127.0.0.1",
					},
					&cli.StringFlag{
						Name:  "port",
						Usage: "Binding port",
						Value: "3333",
					},
					&cli.Uint64Flag{
						Name:  "worker",
						Usage: "Number of worker",
						Value: 3,
					},
				},
				Action: func(c *cli.Context) error {
					bind := c.Value("bind").(string)
					port := c.Value("port").(string)
					worker_num := c.Value("worker").(uint64)

					serve(bind, port, worker_num)
					return nil
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
