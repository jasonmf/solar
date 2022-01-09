package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/elliott-davis/solaredge-go/solaredge"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/jasonmf/cmdutils/env"
	"github.com/jasonmf/cmdutils/fatalif"
)

func main() {
	ctx := context.Background()

	trackerFile := env.Must("TRACKER_FILE")
	token := env.Must("SOLAREDGE_AUTH_TOKEN")
	siteIDstr := env.Must("SOLAREDGE_SITEID")
	influxUser := env.Must("INFLUX_USER")
	influxPass := env.Must("INFLUX_PASS")
	influxURL := env.Must("INFLUX_URL")

	// The solaredge package takes the Site ID as an int64
	siteID, err := strconv.ParseInt(siteIDstr, 10, 64)
	fatalif.Error(err, "parsing site ID")

	// You may optionally include your own http client
	client := solaredge.NewClient(nil, token)

	influxClient := influxdb2.NewClient(influxURL, influxUser+":"+influxPass)
	// The database is solar, autogen is the automatically-created
	// retention period that keeps data forever
	influxWrite := influxClient.WriteAPIBlocking("", "solar/autogen")

	// my location is US Pacific
	timeZone, err := time.LoadLocation("America/Los_Angeles")
	fatalif.Error(err, "loading timezone")

	// if tracker data exists, load it
	tracker := tracker{}
	if b, err := ioutil.ReadFile(trackerFile); err == nil {
		fatalif.Error(json.Unmarshal(b, &tracker), "unmarshalling tracker")
	} else if !os.IsNotExist(err) {
		log.Fatal("error reading file: ", err.Error())
	}

	for {
		// Sleep until 1 minute after the next 15 minute Window
		sleepUntil := time.Now().In(timeZone)
		delayQuarter := 16 - sleepUntil.Minute()%15
		sleepUntil = sleepUntil.Add(time.Minute * time.Duration(delayQuarter)).Truncate(time.Minute)
		log.Println("Sleep Until:", sleepUntil)
		time.Sleep(time.Until(sleepUntil))
		log.Print("checking")

		now := time.Now().In(timeZone)
		before := now.Add(time.Hour * -1)

		tu := solaredge.QuarterOfAnHour

		var resp solaredge.SiteEnergyDetails
		retrySleep := time.Second
		for retries := 5; retries > 0; retries-- {
			// retrieve energy production details for the last hour
			resp, err = client.Site.EnergyDetails(siteID, solaredge.SiteEnergyDetailsRequest{
				StartTime: solaredge.DateTime{Time: before},
				EndTime:   solaredge.DateTime{Time: now},
				TimeUnit:  &tu,
				Meters: []solaredge.Meter{
					solaredge.Production,
				},
			})
			if err != nil {
				log.Printf("error getting site energy: %s", err.Error())
				time.Sleep(retrySleep)
				retrySleep *= 2
			}
		}
		if err != nil {
			log.Print("retries exceeded getting site energy")
			continue
		}

		// tease out the values for Production
		var meter solaredge.Meters
		for _, m := range resp.Meters {
			if m.Type == solaredge.Production {
				meter = m
				break
			}
		}

		// if we didn't get anything, something went wrong
		if len(meter.Values) == 0 {
			log.Fatal("no extracted meter")
		}

		// sort, newest first
		sort.Slice(meter.Values, func(i, j int) bool {
			return meter.Values[i].Date.Time.After(meter.Values[j].Date.Time)
		})

		influxTags := map[string]string{"unit": "energy"}

		// we want to record some of the values
		for _, value := range meter.Values {
			if value.Value == nil {
				// no value ready
				continue
			}
			recorded := false
			if now.Sub(value.Date.Time) > 15*time.Minute && !tracker.Has(value.Date.Time) {
				// this value is older than 15 minutes and hasn't been recorded
				p := influxdb2.NewPoint("energyDetail",
					influxTags,
					map[string]interface{}{
						"generated": *value.Value,
					},
					now,
				)
				err := influxWrite.WritePoint(ctx, p)
				if err != nil {
					log.Printf("error writing record: %s", err.Error())
				} else {
					// successfully recorded in Influx
					recorded = true
					tracker.Add(value.Date.Time)
				}
			}
			log.Printf("%s: %f %v", value.Date.Time, *value.Value, recorded)
		}

		// clean out tracker values older than a day
		tracker.PruneOlder(now.Add(-24 * time.Hour))

		// save a snapshot of the tracker data
		b, err := json.Marshal(tracker)
		fatalif.Error(err, "marshaling tracker")
		fatalif.Error(ioutil.WriteFile(trackerFile, b, 0644), "writing tracker")
	}
}

// tracker is a logical set of int64s, unix timestamps
type tracker map[int64]struct{}

// Has indicates whether the time is in the tracker
func (t tracker) Has(ts time.Time) bool {
	u := ts.Unix()
	_, present := t[u]
	return present
}

// Add a timestamp to the tracker
func (t tracker) Add(ts time.Time) {
	t[ts.Unix()] = struct{}{}
}

// PruneOlder removes all timestamps older than the specified tiem from the set
func (t tracker) PruneOlder(ts time.Time) {
	u := ts.Unix()
	for v := range t {
		if v < u {
			delete(t, v)
		}
	}
}
