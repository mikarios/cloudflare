package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/cloudflare/cloudflare-go"
	"github.com/sirupsen/logrus"
)

var (
	apiToken  = flag.String("token", "", "Your cloudflare API token")
	zoneName  = flag.String("zone", "", "Your zone name. e.g. ikarios.dev")
	subDomain = flag.String("subdomain", "", "The subdomain you wish to create/update. e.g. home")
)

func main() {
	flag.Parse()

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

	existingDNS, err := api.DNSRecords(zoneID, cloudflare.DNSRecord{Name: fqdn})
	if err != nil {
		panic(err)
	}

	if len(existingDNS) == 0 {
		logrus.Info("DNS record not found. Adding it")

		_, err := api.CreateDNSRecord(zoneID, cloudflare.DNSRecord{
			Type:    "A",
			Name:    fqdn,
			Content: myIP,
			Proxied: true,
			TTL:     1,
			ZoneID:  zoneID,
		})
		if err != nil {
			panic(err)
		}

		return
	}

	if existingDNS[0].Content != myIP {
		logrus.Infof("Updating DNS. Old IP: %s, New IP: %s", existingDNS[0].Content, myIP)

		existingDNS[0].Content = myIP
		if err := api.UpdateDNSRecord(zoneID, existingDNS[0].ID, existingDNS[0]); err != nil {
			panic(err)
		}
	}

	logrus.Info("Finished")
}

func findMyIP() (string, error) {
	url := "https://api.ipify.org?format=text"

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		logrus.Error("could not create request", err)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		logrus.Error("could not get site", err)
	}

	defer resp.Body.Close()

	ip, err := ioutil.ReadAll(resp.Body)

	return string(ip), fmt.Errorf("could not parse ip, error: %w", err)
}
