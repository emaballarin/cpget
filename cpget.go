package cpget

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Songmu/prompter"
	"github.com/asaskevich/govalidator"
	"github.com/pkg/errors"
)

// CPget structs
type CPget struct {
	Trace  bool
	Output string
	Procs  int
	URLs   []string

	args      []string
	timeout   int
	useragent string
	referer   string
}

// New for cpget package
func New() *CPget {
	return &CPget{
		Trace:   false,
		Procs:   runtime.NumCPU(), // default
		timeout: 10,
	}
}

// Run execute methods in cpget package
func (cpget *CPget) Run(ctx context.Context, version string, args []string) error {
	if err := cpget.Ready(version, args); err != nil {
		return errTop(err)
	}

	// TODO(codehex): calc maxIdleConnsPerHost
	client := newDownloadClient(16)

	target, err := Check(ctx, &CheckConfig{
		URLs:    cpget.URLs,
		Timeout: time.Duration(cpget.timeout) * time.Second,
		Client:  client,
	})
	if err != nil {
		return err
	}

	filename := target.Filename

	var dir string
	if cpget.Output != "" {
		fi, err := os.Stat(cpget.Output)
		if err == nil && fi.IsDir() {
			dir = cpget.Output
		} else {
			dir, filename = filepath.Split(cpget.Output)
			if dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return errors.Wrapf(err, "failed to create diretory at %s", dir)
				}
			}
		}
	}

	opts := []DownloadOption{
		WithUserAgent(cpget.useragent, version),
		WithReferer(cpget.referer),
	}

	return Download(ctx, &DownloadConfig{
		Filename:      filename,
		Dirname:       dir,
		ContentLength: target.ContentLength,
		Procs:         cpget.Procs,
		URLs:          target.URLs,
		Client:        client,
	}, opts...)
}

const (
	warningNumConnection = 4
	warningMessage       = "[WARNING] Using a large number of connections to 1 URL can lead to DOS attacks.\n" +
		"In most cases, `4` or less is enough. In addition, the case is increasing that if you use multiple connections to 1 URL does not increase the download speed with the spread of CDNs.\n" +
		"See: https://github.com/emaballarin/cpget#disclaimer\n" +
		"\n" +
		"Would you execute knowing these?\n"
)

// Ready method define the variables required to Download.
func (cpget *CPget) Ready(version string, args []string) error {
	opts, err := cpget.parseOptions(args, version)
	if err != nil {
		return errors.Wrap(errTop(err), "failed to parse command line args")
	}

	if opts.Trace {
		cpget.Trace = opts.Trace
	}

	if opts.Timeout > 0 {
		cpget.timeout = opts.Timeout
	}

	if err := cpget.parseURLs(); err != nil {
		return errors.Wrap(err, "failed to parse of url")
	}

	if opts.NumConnection > warningNumConnection && !prompter.YN(warningMessage, false) {
		return makeIgnoreErr()
	}

	cpget.Procs = opts.NumConnection * len(cpget.URLs)

	if opts.Output != "" {
		cpget.Output = opts.Output
	}

	if opts.UserAgent != "" {
		cpget.useragent = opts.UserAgent
	}

	if opts.Referer != "" {
		cpget.referer = opts.Referer
	}

	return nil
}

func (cpget *CPget) parseOptions(argv []string, version string) (*Options, error) {
	var opts Options
	if len(argv) == 0 {
		stdout.Write(opts.usage(version))
		return nil, makeIgnoreErr()
	}

	o, err := opts.parse(argv, version)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse command line options")
	}

	if opts.Help {
		stdout.Write(opts.usage(version))
		return nil, makeIgnoreErr()
	}

	if opts.Update {
		result, err := opts.isupdate(version)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse command line options")
		}

		stdout.Write(result)
		return nil, makeIgnoreErr()
	}

	cpget.args = o

	return &opts, nil
}

func (cpget *CPget) parseURLs() error {

	// find url in args
	for _, argv := range cpget.args {
		if govalidator.IsURL(argv) {
			cpget.URLs = append(cpget.URLs, argv)
		}
	}

	if len(cpget.URLs) < 1 {
		fmt.Fprintf(stdout, "Please input url separate with space or newline\n")
		fmt.Fprintf(stdout, "Start download with ^D\n")

		// scanning url from stdin
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			scan := scanner.Text()
			urls := strings.Split(scan, " ")
			for _, url := range urls {
				if govalidator.IsURL(url) {
					cpget.URLs = append(cpget.URLs, url)
				}
			}
		}

		if err := scanner.Err(); err != nil {
			return errors.Wrap(err, "failed to parse url from stdin")
		}

		if len(cpget.URLs) < 1 {
			return errors.New("urls not found in the arguments passed")
		}
	}

	return nil
}
