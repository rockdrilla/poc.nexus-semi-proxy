package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

var (
	listenAddr   = flag.String("listen", ":80", "listen address")
	nexusRootUri = flag.String("nexus-uri", "https://nexus.svc.domain.com", "Sonatype Nexus root URI")
	nexusRepoFmt = flag.String("repo-name-fmt", "apt-%s-%s", "printf(3) format string for repository name")
)

const (
	// "%s/service/rest/v1/search/assets?repository=%s&q=%s&sort=version&direction=desc"
	fmtNexusSearch string = "%s/service/rest/v1/search/assets?repository=%s&q=%s&sort=version"
)

type nexusAsset struct {
	Id          string `json:"id"`
	Path        string `json:"path"`
	DownloadUrl string `json:"downloadUrl"`
}

type nexusAssetResponse struct {
	Items []nexusAsset `json:"items"`
	Token string       `json:"continuationToken"`
}

func main() {
	flag.Parse()

	http_transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    15 * time.Second,
		DisableCompression: true,
	}

	app := fiber.New(fiber.Config{
		AppName: "apt-nexus-redirector",
	})
	app.Use(logger.New(logger.Config{
		Format:     "${time} ${ip}:${port} ${status} ${method} ${latency} ${path}\n",
		TimeFormat: time.RFC3339,
	}))

	// GET /debian/bookworm/libc6_2.36-9_amd64.deb
	app.Get("/:distro/:suite/:package", func(c *fiber.Ctx) error {
		http_client := &http.Client{Transport: http_transport}

		nexusRepo := fmt.Sprintf(*nexusRepoFmt, c.Params("distro"), c.Params("suite"))
		nexusUriBase := fmt.Sprintf(fmtNexusSearch, *nexusRootUri, nexusRepo, c.Params("package"))

		token := ""
		nexusUri := ""

		for {
			if token != "" {
				nexusUri = fmt.Sprintf("%s&continuationToken=%s", nexusUriBase, token)
			} else {
				nexusUri = nexusUriBase
			}

			resp, err := http_client.Get(nexusUri)
			if err != nil {
				return c.Status(400).SendString(err.Error())
			}

			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return c.Status(400).SendString(err.Error())
			}

			var res *nexusAssetResponse
			err = json.Unmarshal(body, &res)
			if err != nil {
				return c.Status(400).SendString(err.Error())
			}

			if len(res.Items) == 0 {
				break
			}

			for _, x := range res.Items {
				if path.Base(x.Path) == c.Params("package") {
					return c.Redirect(x.DownloadUrl, 302)
				}
			}

			token = res.Token
			if token == "" {
				break
			}
		}

		return c.Status(404).SendString(fmt.Sprintf("%s is not found in %s\n", c.Params("package"), nexusRepo))
	})

	log.Fatal(app.Listen(*listenAddr))
}
