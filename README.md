# benchflux

Store Go benchmark result in InfluxDB

# Usage

## Installation

    go get github.com/nathanielc/benchflux

## Store Benchmark results in InfluxDB

Store results directly:

    go test -run=NONE -bench . ./ | benchflux  \
        -url http://influxdb.example.com:8086 \
        -db benchmarks \
        -rp default \
        -measurement mypackage \
        -source master:$GITSHA

Store results from saved run

    benchflux -url http://influxdb.exmaple.com:8086 benchmark.txt

Set env vars for easier saving of results:

    BF_INFLUXDB_URL='http://influxdb.example.com:8086'
    BF_INFLUXDB_DB=benchmarks
    BF_INFLUXDB_RP=default
    BF_INFLUXDB_MEASUREMENT=mypackage
    BF_SOURCE_NAME=$GITSHA
    go test -run=NONE -bench . ./ | benchflux

# Using with Kapacitor

If you regularly record benchmarks then you can use Kapacitor to compute averages and then show the increase or decrease with changes.

```javascript
// Define a time where all values should be averaged
var agg_period = 1d

// Query raw benchmarks
var data = batch
    |query('''
    SELECT
        mean("iterations") as iterations,
        mean("ns_per_op") as ns_per_op,
        mean("mb_per_s") as mb_per_s,
        mean("alloced_bytes_per_op") as alloced_bytes_per_op,
        mean("allocs_per_op") as allocs_per_op,
    FROM benchmarks..mypackage
''')
        .period(agg_period)
        .every(agg_period)
        .groupBy('benchmark','index','source')
    // Group by benchmark and index
    // so we can aggregate across source.
    |groupBy('benchmark','index')

// Compute the means across sources
var iterations = data
        .as('iterations')
var ns_per_op = data
    |mean('ns_per_op')
        .as('ns_per_op')
var mb_per_s = data
    |mean('mb_per_s')
        .as('mb_per_s')
var alloced_bytes_per_op = data
    |mean('alloced_bytes_per_op')
        .as('alloced_bytes_per_op')
var allocs_per_op = data
    |mean('allocs_per_op')
        .as('allocs_per_op')

// Write summary data to InfluxDB
iterations
    |join(ns_per_op, mb_per_s, alloced_bytes_per_op, allocs_per_op)
        .as('iterations', 'ns_per_op', 'mb_per_s', 'alloced_bytes_per_op', 'allocs_per_op')
    |influxDBOut()
        .database('benchmarks')
        .retentionPolicy('default')
        .measurement('mypackage_summary')
        .precision('s')

// Query summary data and compute period over period differences.

var previous = batch
    |query('''
        SELECT
            iterations,
            ns_per_op,
            mb_per_s,
            alloced_bytes_per_op,
            allocs_per_op
        FROM benchmarks..mypackage_summary
''')
        .period(agg_period)
        .every(agg_period)
        .offset(agg_period)
    |shift(agg_period)

var current = batch
    |query('''
        SELECT
            iterations,
            ns_per_op,
            mb_per_s,
            alloced_bytes_per_op,
            allocs_per_op
        FROM benchmarks..mypackage_summary
''')
        .period(agg_period)
        .every(agg_period)
        .offset(agg_period)
    |shift(agg_period)

previous
    |join(current)
        .as('previous', 'current')
    |eval(
        lambda: "current.iterations" / "previous.iterations",
        lambda: "current.ns_per_op" / "previous.ns_per_op",
        lambda: "current.mb_per_s" / "previous.mb_per_s",
        lambda: "current.alloced_bytes_per_op" / "previous.alloced_bytes_per_op",
        lambda: "current.allocs_per_op" / "previous.allocs_per_op",
    )
        .as(
            'iterations',
            'ns_per_op',
            'mb_per_s',
            'alloced_bytes_per_op',
            'allocs_per_op',
        )
    |influxDBOut()
        .database('benchmarks')
        .retentionPolicy('default')
        .measurement('mypackage_change')
        .precision('s')

```


