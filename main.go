package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cloudflare/cloudflare-go"
	"github.com/sirupsen/logrus"
)

var (
	apiToken  = flag.String("token", "", "Your cloudflare API token")
	zoneName  = flag.String("zone", "", "Your zone name. e.g. ikarios.dev")
	subDomain = flag.String("subdomain", "", "The subdomain you wish to create/update. e.g. home")
	proxied   = flag.Bool("proxied", true, "true/false depending on whether you want cloudflare to proxy your IP address")
)

func main() {
	flag.Parse()

	ctx := context.Background()

	*subDomain = strings.TrimSuffix(*subDomain, *zoneName)
	*subDomain = strings.TrimSuffix(*subDomain, ".")

	fqdn := *subDomain + "." + *zoneName

	myIP, err := findMyIP()
	if err != nil {
		panic(err)
	}

	api, err := cloudflare.NewWithAPIToken(*apiToken)
	if err != nil {
		panic(err)
	}

	zoneID, err := api.ZoneIDByName(*zoneName)
	if err != nil {
		panic(err)
	}

	existingDNS, err := api.DNSRecords(ctx, zoneID, cloudflare.DNSRecord{Name: fqdn})
	if err != nil {
		panic(err)
	}

	if len(existingDNS) == 0 {
		logrus.Info("DNS record not found. Adding it")

		_, err := api.CreateDNSRecord(
			ctx,
			zoneID,
			cloudflare.DNSRecord{
				Type:    "A",
				Name:    fqdn,
				Content: myIP,
				Proxied: proxied,
				TTL:     1,
				ZoneID:  zoneID,
			},
		)
		if err != nil {
			panic(err)
		}

		return
	}

	if existingDNS[0].Content != myIP {
		logrus.Infof("Updating DNS. Old IP: %s, New IP: %s", existingDNS[0].Content, myIP)

		existingDNS[0].Content = myIP
		existingDNS[0].Proxied = proxied

		if err := api.UpdateDNSRecord(ctx, zoneID, existingDNS[0].ID, existingDNS[0]); err != nil {
			panic(err)
		}
	}

	logrus.Info("Finished")
}

func findMyIP() (string, error) {
	url := "https://api.ipify.org?format=text"

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		logrus.Error("could not create request", err)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		logrus.Error("could not get site", err)
	}

	defer resp.Body.Close()

	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("could not parse ip, error: %w", err)
	}

	return string(ip), nil
}
