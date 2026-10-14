// Command reconcile сверяет отчёты Read API appmetrica.online с экспортом AppMetrica (NFR-010, T-055).
package main

import (
	"context"
	"errors"
	"io"
	"os"
)

const (
	MetricActive = "active"
	MetricNew    = "new"
	MetricD1     = "d1"
)

var errNotImplemented = errors.New("not implemented")

type Day struct {
	Date   string
	Active float64
	New    float64
	D1     *float64
}

type Ours struct {
	Active map[string]float64
	New    map[string]float64
	D1     map[string]float64
}

type Check struct {
	Date       string
	Metric     string
	Ours       *float64
	AppMetrica *float64
	Diff       *float64
	Pass       bool
	Note       string
}

type Report struct {
	Checks []Check
}

func (r Report) Pass() bool  { return false }
func (r Report) Failed() int { return 0 }

type Snapshot struct{}

func (s Snapshot) Ours() (Ours, error) { return Ours{}, errNotImplemented }

func ParseAppMetricaCSV(io.Reader) ([]Day, error) { return nil, errNotImplemented }

func Compare([]Day, Ours) Report { return Report{} }

func WriteTable(io.Writer, Report) error { return errNotImplemented }

func run(context.Context, []string, io.Writer, io.Writer, func(string) string) int { return 0 }

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}
