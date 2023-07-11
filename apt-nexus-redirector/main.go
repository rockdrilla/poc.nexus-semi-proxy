package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/valyala/fasthttp"
)

var (
	listenAddr        = flag.String("listen", ":80", "listen address")
	nexusRootUri      = flag.String("nexus-uri", "https://nexus.svc.domain.com", "Sonatype Nexus root URI")
	fmtNexusAptRepo   = flag.String("fmt-repo-apt", "apt-%s-%s", "printf(3) format string for APT packages repository name")
	fmtNexusListsRepo = flag.String("fmt-repo-lists", "raw-lists-%s-%s", "printf(3) format string for lists repository name")

	http_transport *http.Transport
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

	http_transport = &http.Transport{
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

	app.Get("/:distro/:suite/dists/*", routeDists)
	app.Get("/:distro/:suite/pool/*", routePackages)

	log.Fatal(app.Listen(*listenAddr))
}

// GET /debian/bookworm/dists/bookworm/main/binary-amd64/Packages.xz
// > {nexusRootUri}/repository/raw-lists-debian-bookworm/main/binary-amd64/Packages.xz
func routeDists(c *fiber.Ctx) error {
	distro := c.Params("distro")
	distPath := c.Params("*")
	distParts := strings.Split(distPath, "/")
	suite := distParts[0]
	if suite != c.Params("suite") {
		uri := c.Request().URI()

		var _uri fasthttp.URI
		uri.CopyTo(&_uri)
		_uri.SetPath(fmt.Sprintf("/%s/%s/%s", distro, suite, distPath))

		origUri := uri.String()
		newUri := _uri.String()

		c.Set("X-Location-Proposal", newUri)

		return c.Status(400).SendString(fmt.Sprintf("invalid url: %s\nproposed url: %s\n", origUri, newUri))
	}

	distPath = strings.Join(distParts[1:], "/")
	nexusRepo := fmt.Sprintf(*fmtNexusListsRepo, distro, suite)
	nexusUri := fmt.Sprintf("%s/repository/%s/%s", *nexusRootUri, nexusRepo, distPath)
	return c.Redirect(nexusUri, 307)
}

// GET /debian/bookworm/pool/l/libc6/libc6_2.36-9_amd64.deb
// > {nexusRootUri}/repository/apt-debian-bookworm/pool/l/libc6/libc6_2.36-9_amd64.deb
func routePackages(c *fiber.Ctx) error {
	packagePath := c.Params("*")

	if strings.HasSuffix(packagePath, "/") {
		return c.Status(400).SendString(fmt.Sprintf("invalid path: %s\n", packagePath))
	}

	http_client := &http.Client{Transport: http_transport}

	nexusRepo := fmt.Sprintf(*fmtNexusAptRepo, c.Params("distro"), c.Params("suite"))
	packageFileName := path.Base(packagePath)
	nexusUriBase := fmt.Sprintf(fmtNexusSearch, *nexusRootUri, nexusRepo, packageFileName)

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
			if path.Base(x.Path) == packageFileName {
				return c.Redirect(x.DownloadUrl, 302)
			}
		}

		token = res.Token
		if token == "" {
			break
		}
	}

	return c.Status(404).SendString(fmt.Sprintf("%s is not found in %s\n", packageFileName, nexusRepo))
}
