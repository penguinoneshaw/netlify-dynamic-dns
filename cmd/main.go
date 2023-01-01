package main

import (
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alexflint/go-arg"
	openApiRuntime "github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/janeczku/go-spinner"
	"github.com/netlify/open-api/go/models"
	"github.com/netlify/open-api/go/plumbing/operations"
	"github.com/netlify/open-api/go/porcelain"
	"github.com/oscartbeaumont/netlify-dynamic-dns/internal/publicip"
	"golang.org/x/exp/slices"
)

var args Arguments
var netlify = porcelain.NewRetryable(porcelain.Default.Transport, nil, porcelain.DefaultRetryAttempts)
var netlifyAuth = openApiRuntime.ClientAuthInfoWriterFunc(func(r openApiRuntime.ClientRequest, _ strfmt.Registry) error {
	if err := r.SetHeaderParam("User-Agent", "NetlifyDDNS"); err != nil {
		return err
	}
	if err := r.SetHeaderParam("Authorization", "Bearer "+args.AccessToken); err != nil {
		return err
	}
	return nil
})

func constructRecords(args Arguments) []string {
	if args.UpdateRootRecord {
		return []string{args.Zone}
	}

	result := make([]string, len(args.Record))

	for index, record := range args.Record {
		result[index] = record + "." + args.Zone
	}

	return result
}

func main() {
	validation := arg.MustParse(&args)
	zoneId := strings.ReplaceAll(args.Zone, ".", "_")

	if !args.UpdateRootRecord && len(args.Record) == 0 {
		validation.Fail("Either --record or --updaterootrecord must be provided")
	}

	var continueUpdating = true
	var waitInterval = time.Duration(args.Interval) * time.Minute
	var backOff = 0

	ipProvider, err := publicip.NewOpenDNSProvider("1.1.1.1:53")
	if err != nil {
		log.Fatalf("Failed to initialize OpenDNS provider with error: %v\n", err)
	}

	for continueUpdating {
		s := spinner.StartNew("Updating DNS record")
		err := doUpdate(ipProvider, zoneId, constructRecords(args))
		s.Stop()

		if err != nil {
			log.Println(Red + "Error: Updating DNS Record failed with error: " + err.Error() + Reset)
			backOffTime := (1 << backOff) * time.Second
			backOff += 1
			log.Printf("Retrying in %d seconds\n", backOffTime)

		} else if args.Interval == 0 {
			log.Println(Green + "DNS records updated successfully." + Reset + "")
			continueUpdating = false
		} else {
			log.Println(Green + "DNS records updated successfully. Next update in " + strconv.Itoa(args.Interval) + " minutes" + Reset + "")
			backOff = 0
			time.Sleep(waitInterval)
		}
	}
}

// doUpdate updates the DNS records with the public IP address
func doUpdate(ipProvider publicip.Provider, zoneID string, records []string) error {
	// Get the Public IP
	ipv4, err := ipProvider.GetIPv4()

	if err != nil {
		return fmt.Errorf("error retrieving your public ipv4 address: %w", err)
	}

	var ipv6 string
	if args.IPv6 {
		ipv6, err = ipProvider.GetIPv6()
		if err != nil {
			return fmt.Errorf("error retrieving your public ipv6 address: %w", err)
		}
	}

	getparams := operations.NewGetDNSRecordsParams().WithZoneID(zoneID)
	resp, err := netlify.Operations.GetDNSRecords(getparams, netlifyAuth)
	if err != nil {
		errr, apiError := err.(*operations.GetDNSRecordsDefault)
		if apiError && errr.Code() == http.StatusUnauthorized {
			log.Fatalln("\r" + Red + "Fatal Error: Netlify API access token unauthorised" + Reset + "")
		} else {
			return fmt.Errorf("error retrieving your records from Netlify DNS: %w", err)
		}
	}

	numCPU := runtime.NumCPU()
	c := make(chan error, numCPU)
	defer close(c)

	deleteRecord := func(waitGroup *sync.WaitGroup, record *models.DNSRecord) {
		defer waitGroup.Done()

		if record != nil {
			deleteparams := operations.NewDeleteDNSRecordParams()
			deleteparams.ZoneID = record.DNSZoneID
			deleteparams.DNSRecordID = record.ID
			if _, err := netlify.Operations.DeleteDNSRecord(deleteparams, netlifyAuth); err != nil {
				c <- err
			}
		}
	}

	var waitGroup sync.WaitGroup

	for _, record := range resp.Payload {
		if record.Type == "A" || record.Type == "AAAA" && args.IPv6 {
			if slices.Contains(records, record.Hostname) {
				waitGroup.Add(1)
				go deleteRecord(&waitGroup, record)
			}
		}
	}

	waitGroup.Wait()

	if len(c) > 0 {
		return fmt.Errorf("error creating new DNS record on Netlify DNS: %w", <-c)
	}

	createRecord := func(c chan<- error, waitGroup *sync.WaitGroup, recordType string, record string, value string, ttl int64) {
		defer waitGroup.Done()
		var newRecord = &models.DNSRecordCreate{
			Hostname: record,
			Type:     recordType,
			Value:    value,
			TTL:      ttl,
		}
		createparams := operations.NewCreateDNSRecordParams().WithZoneID(zoneID).WithDNSRecord(newRecord)

		if _, err := netlify.Operations.CreateDNSRecord(createparams, netlifyAuth); err != nil {
			c <- err
		}
	}

	for _, record := range records {
		var ttl int64 = 3600
		if args.Interval > 0 {
			ttl = int64(args.Interval + 30)
		}
		waitGroup.Add(1)
		go createRecord(c, &waitGroup, "A", record, ipv4, ttl)

		if args.IPv6 {
			waitGroup.Add(1)
			go createRecord(c, &waitGroup, "AAAA", record, ipv6, ttl)
		}
	}
	waitGroup.Wait()

	if len(c) > 0 {
		return <-c
	}

	return nil
}
