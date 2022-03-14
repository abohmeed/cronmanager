package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/juju/fslock"
)

//isDelayed: Used to signal that the cron job delay was triggered
var (
	isDelayed    = false
	jobStartTime time.Time
	jobDuration  float64
	flgVersion   bool
	version      string
)

const (
	idleForSeconds = 60
)

// wait for rest of the idleForSeconds so Prometheus can notice that something is happening
func idleWait(jobStart time.Time) {
	// Idling to let Prometheus to notice we are running
	diff := idleForSeconds - (time.Now().Unix() - jobStart.Unix())
	if diff > 0 {
		fmt.Printf("Idle flag active so I am going to wait for for additional %d seconds", diff)
		time.Sleep(time.Second * time.Duration(diff))
	}
}

func main() {

	version = "1.1.18"
	idle := flag.Bool("i", false, fmt.Sprintf("Idle for %d seconds at the beginning so Prometheus can notice it's actually running", idleForSeconds))
	cmdPtr := flag.String("c", "", "[Required] The `cron job` command")
	jobnamePtr := flag.String("n", "", "[Required] The `job name` to appear in the alarm")
	logfilePtr := flag.String("l", "", "[Optional] The `log file` to store the cron output")
	flag.BoolVar(&flgVersion, "version", false, "if true print version and exit")
	flag.Parse()
	if flgVersion {
		fmt.Println("CronManager version " + version)
		os.Exit(0)
	}
	flag.Usage = func() {
		fmt.Printf("Usage: cronmanager -c command  -n jobname  [ -l log file ]\nExample: cronmanager \"/usr/bin/php /var/www/app.zlien.com/console broadcast:entities:updated -e project -l 20000\" -n update_entitites_cron -t 3600 -l /path/to/log\n")
		flag.PrintDefaults()
	}
	if *cmdPtr == "" || *jobnamePtr == "" {
		flag.Usage()
		os.Exit(1)
	}

	//Record the start time of the job
	jobStartTime = time.Now()
	//Start a ticker in a goroutine that will write an alarm metric if the job exceeds the time
	go func() {
		for range time.Tick(time.Second) {
			jobDuration = time.Since(jobStartTime).Seconds()
			// Log current duration counter
			writeToExporter(*jobnamePtr, "duration", strconv.FormatFloat(jobDuration, 'f', 0, 64))
			// Store last timestamp
			writeToExporter(*jobnamePtr, "last", fmt.Sprintf("%d", time.Now().Unix()))
		}
	}()

	// Job started
	writeToExporter(*jobnamePtr, "run", "1")

	// Parse the command by extracting the first token as the command and the rest as its args
	cmdArr := strings.Split(*cmdPtr, " ")
	cmdBin := cmdArr[0]
	cmdArgs := cmdArr[1:]
	cmd := exec.Command(cmdBin, cmdArgs...)

	var buf bytes.Buffer

	// If we have a log file specified, use it
	if *logfilePtr != "" {
		outfile, err := os.Create(*logfilePtr)
		if err != nil {
			panic(err)
		}
		defer outfile.Close()
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			panic(err)
		}
		writer := bufio.NewWriter(outfile)
		defer writer.Flush()
		go io.Copy(writer, stdoutPipe)
	} else {
		cmd.Stdout = &buf
	}
	err := cmd.Start()
	if err != nil {
		log.Fatal(err)
	}

	// Execute the command
	err = cmd.Wait()

	// wait if idle is active
	if *idle {
		idleWait(jobStartTime)
	}

	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if _, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				writeToExporter(*jobnamePtr, "failed", "1")
				// Job is no longer running
				writeToExporter(*jobnamePtr, "run", "0")
			}
		} else {
			log.Fatalf("cmd.Wait: %v", err)
		}
	} else {
		// The job had no errors
		writeToExporter(*jobnamePtr, "failed", "0")
		// Job is no longer running
		writeToExporter(*jobnamePtr, "run", "0")
		// In all cases, unlock the file
	}

	// Store last timestamp
	writeToExporter(*jobnamePtr, "last", fmt.Sprintf("%d", time.Now().Unix()))
}

func getExporterPath() string {
	exporterPath, exists := os.LookupEnv("COLLECTOR_TEXTFILE_PATH")
	exporterPath = exporterPath + "/crons.prom"
	if !exists {
		exporterPath = "/var/cache/prometheus/crons.prom"
	}
	return exporterPath
}

func writeToExporter(jobName string, label string, metric string) {
	jobNeedle := "cronjob{name=\"" + jobName + "\",dimension=\"" + label + "\"}"
	typeData := "# TYPE cron_job gauge"
	jobData := jobNeedle + " " + metric

	exporterPath := getExporterPath()
	// Lock filepath to prevent race conditions
	lock := fslock.New(exporterPath)
	err := lock.Lock()
	if err != nil {
		log.Println("Error locking file " + exporterPath)
	}
	defer lock.Unlock()

	input, err := ioutil.ReadFile(exporterPath)
	if err != nil {
		// We're not sure why we can't read from the file. Let's try creating it and fail if that didn't work either
		if _, err := os.Create(exporterPath); err != nil {
			log.Fatal("Couldn't read or write to the exporter file. Check parent directory permissions")
		}
	}
	re := regexp.MustCompile(jobNeedle + `.*\n`)
	// If we have the job data alrady, just replace it and that's it
	if re.Match(input) {
		input = re.ReplaceAll(input, []byte(jobData+"\n"))
	} else {
		// If the job is not there then either there is no TYPE header at all and this is the first job
		if re := regexp.MustCompile(typeData); !re.Match(input) {
			// Add the TYPE and the job data
			input = append(input, typeData+"\n"...)
			input = append(input, jobData+"\n"...)
		} else {
			// Or there is a TYPE header with one or more other jobs. Just append the job to the TYPE header
			input = re.ReplaceAll(input, []byte(typeData+"\n"+jobData))
		}
	}
	f, err := os.Create(exporterPath)
	if err != nil {
		log.Fatal(err)
	}
	err = f.Chmod(0644)
	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()
	if _, err = f.Write(input); err != nil {
		log.Fatal(err)
	}
}
