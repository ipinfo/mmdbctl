package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"reflect"

	"github.com/ipinfo/cli/lib/complete"
	"github.com/ipinfo/cli/lib/complete/predict"
	"github.com/oschwald/maxminddb-golang"
	"github.com/spf13/pflag"
)

type cmdDiffRecord struct {
	oldr interface{}
	newr interface{}
}

var completionsDiff = &complete.Command{
	Flags: map[string]complete.Predictor{
		"-h":        predict.Nothing,
		"--help":    predict.Nothing,
		"-s":        predict.Nothing,
		"--subnets": predict.Nothing,
		"-r":        predict.Nothing,
		"--records": predict.Nothing,
	},
}

func printHelpDiff() {
	fmt.Printf(
		`Usage: %s diff [<opts>] <old> <new>

Description:
  Print subnet and record differences between two mmdb files (i.e. do set
  difference `+"`"+"(new - old) U (old - new)"+"`"+`).

Options:
  General:
    --help, -h
      show help.
    --subnets, -s
      show subnets difference.
    --records, -r
      show records difference.
`, progBase)
}

func doDiff(
	newDb *maxminddb.Reader,
	newDbStr string,
	oldDb *maxminddb.Reader,
	oldDbStr string,
) (map[interface{}]*net.IPNet, map[interface{}]cmdDiffRecord, error) {
	modifiedSubnets := map[interface{}]*net.IPNet{}
	modifiedRecords := map[interface{}]cmdDiffRecord{}
	networksA := newDb.Networks(maxminddb.SkipAliasedNetworks)
	for networksA.Next() {
		var recordA interface{}
		var recordB interface{}

		subnetA, err := networksA.Network(&recordA)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"failed to get record for subnet from %v: %w",
				newDbStr, err,
			)
		}

		subnetB, _, err := oldDb.LookupNetwork(subnetA.IP, &recordB)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"failed to get record for IP %v from %v: %w",
				subnetA.IP, oldDbStr, err,
			)
		}

		// unequal subnets?
		if bytes.Compare(subnetA.IP, subnetB.IP) != 0 ||
			bytes.Compare(subnetA.Mask, subnetB.Mask) != 0 {
			modifiedSubnets[subnetA] = subnetB
			continue
		}

		// different data for same subnet?
		if !reflect.DeepEqual(recordA, recordB) {
			modifiedRecords[subnetA] = cmdDiffRecord{
				oldr: recordB,
				newr: recordA,
			}
		}
	}
	if networksA.Err() != nil {
		return nil, nil, fmt.Errorf(
			"failed traversing networks of %v: %w",
			newDbStr, networksA.Err(),
		)
	}

	return modifiedSubnets, modifiedRecords, nil
}

func cmdDiff() error {
	var fSubnets bool
	var fRecords bool

	_h := "see description in --help"
	pflag.BoolVarP(&fSubnets, "subnets", "s", false, _h)
	pflag.BoolVarP(&fRecords, "records", "r", false, _h)
	pflag.Parse()

	if fHelp || (pflag.NArg() == 1 && pflag.NFlag() == 0) {
		printHelpDiff()
		return nil
	}

	// get args excluding subcommand.
	args := pflag.Args()[1:]

	// validate input files.
	if len(args) != 2 {
		return errors.New("two input mmdb file required as arguments")
	}

	// open old db.
	oldMmdb := args[0]
	oldDb, err := maxminddb.Open(oldMmdb)
	if err != nil {
		return fmt.Errorf("couldnt open %v: %w", oldMmdb, err)
	}
	defer oldDb.Close()

	// open new db.
	newMmdb := args[1]
	newDb, err := maxminddb.Open(newMmdb)
	if err != nil {
		return fmt.Errorf("couldnt open %v: %w", newMmdb, err)
	}
	defer newDb.Close()

	// confirm that they're of the same IP version.
	if newDb.Metadata.IPVersion != oldDb.Metadata.IPVersion {
		return fmt.Errorf(
			"IP versions differ between files: %v=%v and %v=%v",
			newMmdb, newDb.Metadata.IPVersion,
			oldMmdb, oldDb.Metadata.IPVersion,
		)
	}

	// collect set difference data.
	ambSn, ambRec, err := doDiff(newDb, newMmdb, oldDb, oldMmdb)
	if err != nil {
		return err
	}
	bmaSn, _, err := doDiff(oldDb, oldMmdb, newDb, newMmdb)
	if err != nil {
		return err
	}

	// print.
	if fSubnets {
		if len(ambSn) > 0 || len(bmaSn) > 0 {
			fmt.Println("** SUBNETS **")
			for newSn, oldSn := range ambSn {
				fmt.Printf("%v -> %v\n", oldSn, newSn)
			}
			for newSn, oldSn := range bmaSn {
				fmt.Printf("%v -> %v\n", newSn, oldSn)
			}
		}
		fmt.Println(len(ambSn) + len(bmaSn), "subnet(s) modified.")
	}
	if fRecords {
		if fSubnets {
			fmt.Println()
		}

		if len(ambRec) > 0 {
			fmt.Println("** RECORDS **")
			for sn, diffRecord := range ambRec {
				fmt.Println(sn)
				fmt.Printf("	-%v\n", diffRecord.oldr)
				fmt.Printf("	+%v\n", diffRecord.newr)
			}
		}
		fmt.Println(len(ambRec), "record(s) modified.")
	}
	if !fSubnets && !fRecords {
		fmt.Println(len(ambSn) + len(bmaSn), "subnet(s) modified.")
		fmt.Println(len(ambRec), "record(s) modified.")
	}

	return nil
}
