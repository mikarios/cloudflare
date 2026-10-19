package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cloudflare/cloudflare-go/v7"
	"github.com/cloudflare/cloudflare-go/v7/dns"
	"github.com/cloudflare/cloudflare-go/v7/option"
	"github.com/cloudflare/cloudflare-go/v7/zones"
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

	if *apiToken == "" || *zoneName == "" {
		flag.Usage()
		os.Exit(2)
	}

	ctx := context.Background()

	*subDomain = strings.TrimSuffix(*subDomain, *zoneName)
	*subDomain = strings.TrimSuffix(*subDomain, ".")

	fqdn := *subDomain + "." + *zoneName

	httpClient := &http.Client{Timeout: 10 * time.Second}
	myIP, err := findMyIP(httpClient, "https://api.ipify.org?format=text")
	if err != nil {
		logrus.WithError(err).Fatal("could not determine public IP")
	}

	api := cloudflare.NewClient(option.WithAPIToken(*apiToken))

	zonesPage, err := api.Zones.List(ctx, zones.ZoneListParams{
		Name: cloudflare.F(*zoneName),
	})
	if err != nil {
		logrus.WithError(err).Fatal("could not look up zone")
	}
	if len(zonesPage.Result) == 0 {
		logrus.WithField("zone", *zoneName).Fatal("zone not found")
	}
	zoneID := zonesPage.Result[0].ID

	recordsPage, err := api.DNS.Records.List(ctx, dns.RecordListParams{
		ZoneID: cloudflare.F(zoneID),
		Name:   cloudflare.F(dns.RecordListParamsName{Exact: cloudflare.F(fqdn)}),
	})
	if err != nil {
		logrus.WithError(err).Fatal("could not list DNS records")
	}

	if len(recordsPage.Result) == 0 {
		logrus.Info("DNS record not found. Adding it")

		_, err := api.DNS.Records.New(ctx, dns.RecordNewParams{
			ZoneID: cloudflare.F(zoneID),
			Body: dns.ARecordParam{
				Name:    cloudflare.F(fqdn),
				TTL:     cloudflare.F(dns.TTL1),
				Type:    cloudflare.F(dns.ARecordTypeA),
				Content: cloudflare.F(myIP),
				Proxied: cloudflare.F(*proxied),
			},
		})
		if err != nil {
			logrus.WithError(err).Fatal("could not create DNS record")
		}

		return
	}

	if recordsPage.Result[0].Content != myIP {
		logrus.Infof("Updating DNS. Old IP: %s, New IP: %s", recordsPage.Result[0].Content, myIP)

		_, err := api.DNS.Records.Update(ctx, recordsPage.Result[0].ID, dns.RecordUpdateParams{
			ZoneID: cloudflare.F(zoneID),
			Body: dns.ARecordParam{
				Name:    cloudflare.F(fqdn),
				TTL:     cloudflare.F(dns.TTL1),
				Type:    cloudflare.F(dns.ARecordTypeA),
				Content: cloudflare.F(myIP),
				Proxied: cloudflare.F(*proxied),
			},
		})
		if err != nil {
			logrus.WithError(err).Fatal("could not update DNS record")
		}
	}

	logrus.Info("Finished")
}

func findMyIP(client *http.Client, url string) (string, error) {
	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("could not create request: %w", err)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("could not get site: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("could not read ip response: %w", err)
	}

	parsed := net.ParseIP(strings.TrimSpace(string(body)))
	if parsed == nil {
		return "", fmt.Errorf("could not parse ip from response %q", string(body))
	}

	return parsed.String(), nil
}
