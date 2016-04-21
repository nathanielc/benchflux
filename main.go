package main

import (
	"flag"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	client "github.com/influxdata/influxdb/client/v2"
	"golang.org/x/tools/benchmark/parse"
)

const (
	urlENVOverride    = "BF_INFLUXDB_URL"
	dbENVOverride     = "BF_INFLUXDB_DB"
	rpENVOverride     = "BF_INFLUXDB_RP"
	mENVOverride      = "BF_INFLUXDB_MEASUREMENT"
	sourceENVOverride = "BF_SOURCE_NAME"
)

var sourceName = flag.String("source", "stdin", "A source name to give the results.")
var influxDBURL = flag.String("url", "http://localhost:8086", "The URL of the InfluxDB host.")
var database = flag.String("db", "benchmarks", "The InfluxDB database name.")
var retentionPolicy = flag.String("rp", "default", "The InfluxDB retention policy name.")
var measurement = flag.String("measurement", "", "The InfluxDB measurement name.")
var nowVar = flag.String("now", "", "The time to use when writing the results. If empty uses current time. RFC3339 format")

func main() {
	flag.Parse()

	if url := os.Getenv(urlENVOverride); url != "" {
		*influxDBURL = url
	}
	if db := os.Getenv(dbENVOverride); db != "" {
		*database = db
	}
	if rp := os.Getenv(rpENVOverride); rp != "" {
		*retentionPolicy = rp
	}
	if m := os.Getenv(mENVOverride); m != "" {
		*measurement = m
	}
	if s := os.Getenv(sourceENVOverride); s != "" {
		*sourceName = s
	}

	inputs, err := determineInputs(*sourceName, flag.Args())
	if err != nil {
		log.Fatal(err)
	}

	now, err := determineTime(*nowVar)
	if err != nil {
		log.Fatal(err)
	}

	benchmarks, err := parseBenchmarks(inputs)
	if err != nil {
		log.Fatal(err)
	}

	err = writeBenchmarks(
		*influxDBURL,
		*database,
		*retentionPolicy,
		*measurement,
		benchmarks,
		now,
	)
	if err != nil {
		log.Fatal(err)
	}

}

func determineInputs(sourceName string, args []string) (map[string]io.ReadCloser, error) {
	var inputs map[string]io.ReadCloser
	if l := len(args); l == 0 {
		inputs = map[string]io.ReadCloser{
			sourceName: os.Stdin,
		}
	} else {
		inputs = make(map[string]io.ReadCloser, l)
		for _, f := range args {
			f, err := os.Open(f)
			if err != nil {
				return nil, err
			}
			inputs[f.Name()] = f
		}
	}
	return inputs, nil
}

func determineTime(nowStr string) (now time.Time, err error) {
	if nowStr != "" {
		now, err = time.Parse(time.RFC3339, nowStr)
	} else {
		now = time.Now()
	}
	return
}

func parseBenchmarks(inputs map[string]io.ReadCloser) (map[string]parse.Set, error) {
	sets := make(map[string]parse.Set, len(inputs))
	for source, in := range inputs {
		set, err := parse.ParseSet(in)
		if err != nil {
			return nil, err
		}
		sets[source] = set
		in.Close()
	}
	return sets, nil
}

func writeBenchmarks(url, db, rp, m string, benchmarks map[string]parse.Set, now time.Time) error {
	cli, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:      url,
		UserAgent: "benchflux",
	})
	if err != nil {
		return err
	}

	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Precision:       "s",
		Database:        db,
		RetentionPolicy: rp,
	})
	if err != nil {
		return err
	}

	for source, set := range benchmarks {
		for name, list := range set {
			for i, benchmark := range list {
				tags := map[string]string{
					"source":    source,
					"benchmark": name,
					"index":     strconv.FormatInt(int64(i), 10),
				}
				fields := make(map[string]interface{}, 5)
				fields["iterations"] = int64(benchmark.N)
				if benchmark.Measured&parse.NsPerOp != 0 {
					fields["ns_per_op"] = benchmark.NsPerOp
				}
				if benchmark.Measured&parse.MBPerS != 0 {
					fields["mb_per_s"] = benchmark.MBPerS
				}
				if benchmark.Measured&parse.AllocedBytesPerOp != 0 {
					fields["alloced_bytes_per_op"] = int64(benchmark.AllocedBytesPerOp)
				}
				if benchmark.Measured&parse.AllocsPerOp != 0 {
					fields["allocs_per_op"] = int64(benchmark.AllocsPerOp)
				}

				p, err := client.NewPoint(
					m,
					tags,
					fields,
					now,
				)
				if err != nil {
					return err
				}
				bp.AddPoint(p)
			}
		}
	}

	return cli.Write(bp)
}
