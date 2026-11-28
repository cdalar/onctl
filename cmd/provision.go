package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cdalar/onctl/internal/ui"
	"github.com/cdalar/onctl/pkg/box"
)

// parseUploads is -u's values: LOCAL:REMOTE, or LOCAL alone for its base
// name in the login directory.
func parseUploads(specs []string) []box.Transfer {
	var out []box.Transfer
	for _, spec := range specs {
		local, remote, ok := strings.Cut(spec, ":")
		if !ok {
			remote = filepath.Base(spec)
		}
		out = append(out, box.Transfer{Local: local, Remote: remote})
	}
	return out
}

// parseDownloads is -d's values: REMOTE:LOCAL, or REMOTE alone for its
// base name in the current directory.
func parseDownloads(specs []string) []box.Transfer {
	var out []box.Transfer
	for _, spec := range specs {
		remote, local, ok := strings.Cut(spec, ":")
		if !ok {
			local = filepath.Base(spec)
		}
		out = append(out, box.Transfer{Local: local, Remote: remote})
	}
	return out
}

// printProgress shows box.Provision's steps as onctl always has, with s
// spinning while a step runs (except apply files, whose own output
// streams to the terminal) and a bar for each upload.
func printProgress(s *ui.Spinner, setup *box.Setup) {
	var started time.Time
	setup.Progress = func(st box.Step) {
		switch st.Kind {
		case box.StepDomain:
			if !st.Done {
				s.Suffix = " Requesting Domain..."
				s.Restart()
				return
			}
			s.Stop()
			if st.Err != nil {
				fmt.Println("\033[31m✘\033[0m Error on Domain: " + st.Err.Error())
			} else {
				fmt.Println("\033[32m✔\033[0m Domain is ready: " + st.Item)
			}
		case box.StepReady:
			if !st.Done {
				s.Suffix = " Waiting for VM to be ready..."
				s.Restart()
				return
			}
			s.Stop()
			if st.Err == nil {
				fmt.Println("\033[32m✔\033[0m VM is Ready")
			}
		case box.StepUpload:
			if !st.Done {
				s.Stop()
				started = time.Now()
				what, size := "file", int64(0)
				if info, err := os.Stat(st.Transfer.Local); err == nil {
					size = info.Size()
					if info.IsDir() {
						what = "directory"
						size, _ = dirSize(st.Transfer.Local)
					}
				}
				fmt.Printf("Uploading %s: %s -> %s (%.1f MB)\n", what, st.Transfer.Local, st.Transfer.Remote, float64(size)/(1024*1024))
				return
			}
			if st.Err != nil {
				fmt.Print("\n")
				log.Printf("[ERROR] Failed to upload %s: %v", st.Transfer.Local, st.Err)
				return
			}
			fmt.Printf("\n\033[32m✔\033[0m Uploaded %s\n", st.Transfer.Remote)
		case box.StepApply:
			if !st.Done {
				s.Stop()
				fmt.Println("Running " + st.Item + " on Remote...")
				return
			}
			if st.Err != nil {
				log.Println(st.Err)
			}
			fmt.Println("\033[32m✔\033[0m " + st.Item + " ran on Remote")
		case box.StepDownload:
			if !st.Done {
				log.Printf("Downloading file: %s -> %s", st.Transfer.Remote, st.Transfer.Local)
				return
			}
			if st.Err != nil {
				log.Printf("[ERROR] Failed to download %s: %v", st.Transfer.Remote, st.Err)
			}
		}
	}
	setup.UploadProgress = func(_ box.Transfer, current, total int64) {
		if total == 0 {
			return
		}
		const width = 20
		filled := int(float64(width) * float64(current) / float64(total))
		if filled > width {
			filled = width
		}
		var mbps float64
		if secs := time.Since(started).Seconds(); secs > 0 {
			mbps = float64(current) / secs / (1024 * 1024)
		}
		fmt.Printf("\r[%s] %5.1f%% (%.1f/%.1f MB) %.1f MB/s", strings.Repeat("=", filled)+strings.Repeat(".", width-filled),
			float64(current)/float64(total)*100, float64(current)/(1024*1024), float64(total)/(1024*1024), mbps)
	}
}
