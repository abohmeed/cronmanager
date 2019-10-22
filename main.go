package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const exporterPath string = "/opt/prometheus/exporters/dist/textfile/"
//isDelayed: Used to signal that the cron job delay was triggered
var isDelayed bool = false

func main() {
	cmdPtr := flag.String("c", "", "[Required] The `cron job` command")
	jobnamePtr := flag.String("n", "", "[Required] The `job name` to appear in the alarm")
	thresPtr := flag.Int("t", 3600, "[Optional] The maximum `time` for this cron to run in seconds. Defaults to 1 hour")
	logfilePtr := flag.String("l", "", "[Optional] The `log file` to store the cron output")
	flag.Parse()
	flag.Usage = func() {
		fmt.Printf("Usage: cronmanager -c command  -n jobname  [ -t time in seconds ] [ -l log file ]\nExample: cronmanager \"/usr/bin/php /var/www/app.zlien.com/console broadcast:entities:updated -e project -l 20000\" -n update_entitites_cron -t 3600 -l /path/to/log\n")
		flag.PrintDefaults()
	}
	if *cmdPtr == "" || *jobnamePtr == "" {
		flag.Usage()
		os.Exit(1)
	}
	// Delete the exporter file if it exists
	prepareExporterFile(*jobnamePtr)
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
	//Start a timer in a goroutine that will write an alarm metric if the job exceeds the time
	ticker := time.NewTicker(time.Second)
	timer := time.NewTimer(time.Second * time.Duration(*thresPtr))
	go func(timer *time.Timer, ticker *time.Ticker) {
		for range timer.C {
			writeToExporter(*jobnamePtr, "{issue=\"delayed\"} 1\n")
			ticker.Stop()
			isDelayed = true
		}
	}(timer, ticker)
	// Execute the command
	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if _, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				writeToExporter(*jobnamePtr, "{issue=\"failed\"} 1\n")
				if !isDelayed {
					writeToExporter(*jobnamePtr, "{issue=\"delayed\"} 0\n")
				}
			}
		} else {
			log.Fatalf("cmd.Wait: %v", err)
		}
	} else {
		// The job had no errors
		writeToExporter(*jobnamePtr, "{issue=\"failed\"} 0\n")
		// and no delay was already logged
		if !isDelayed {
			writeToExporter(*jobnamePtr, "{issue=\"delayed\"} 0\n")
		}
	}
}

func prepareExporterFile(jobName string) {
	targetFile := exporterPath + jobName + ".prom"
	f, err := os.Create(targetFile)
	if err != nil {
		log.Fatal(err)
		return
	}
	_, err = f.WriteString("# TYPE " + jobName + " gauge\n")
	if err != nil {
		log.Fatal(err)
		f.Close()
		return
	}
}

func writeToExporter(jobName string, data string) {
	f, err := os.OpenFile(exporterPath+jobName+".prom",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(jobName + data); err != nil {
		log.Fatal(err)
	}
}
