/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/go-ini/ini"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

const (
	GiB                  = 1024 * 1024 * 1024
	TiB                  = 1024 * GiB
	tagsDelimiter        = ","
	tagKeyValueDelimiter = "="
)

// RoundUpBytes rounds up the volume size in bytes up to multiplications of GiB
// in the unit of Bytes
func RoundUpBytes(volumeSizeBytes int64) int64 {
	return roundUpSize(volumeSizeBytes, GiB) * GiB
}

// RoundUpGiB rounds up the volume size in bytes up to multiplications of GiB
// in the unit of GiB
func RoundUpGiB(volumeSizeBytes int64) int64 {
	return roundUpSize(volumeSizeBytes, GiB)
}

// BytesToGiB conversts Bytes to GiB
func BytesToGiB(volumeSizeBytes int64) int64 {
	return volumeSizeBytes / GiB
}

// GiBToBytes converts GiB to Bytes
func GiBToBytes(volumeSizeGiB int64) int64 {
	return volumeSizeGiB * GiB
}

// roundUpSize calculates how many allocation units are needed to accommodate
// a volume of given size. E.g. when user wants 1500MiB volume, while AWS EBS
// allocates volumes in gibibyte-sized chunks,
// RoundUpSize(1500 * 1024*1024, 1024*1024*1024) returns '2'
// (2 GiB is the smallest allocatable volume that can hold 1500MiB)
func roundUpSize(volumeSizeBytes int64, allocationUnitBytes int64) int64 {
	roundedUp := volumeSizeBytes / allocationUnitBytes
	if volumeSizeBytes%allocationUnitBytes > 0 {
		roundedUp++
	}
	return roundedUp
}

// GetMountOptions return options with string list separated by space
func GetMountOptions(options []string) string {
	if len(options) == 0 {
		return ""
	}
	str := options[0]
	for i := 1; i < len(options); i++ {
		str = str + " " + options[i]
	}
	return str
}

func MakeDir(pathname string, perm os.FileMode) error {
	err := os.MkdirAll(pathname, perm)
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	return nil
}

// LockMap used to lock on entries
type LockMap struct {
	sync.Mutex
	mutexMap map[string]*sync.Mutex
}

// NewLockMap returns a new lock map
func NewLockMap() *LockMap {
	return &LockMap{
		mutexMap: make(map[string]*sync.Mutex),
	}
}

// LockEntry acquires a lock associated with the specific entry
func (lm *LockMap) LockEntry(entry string) {
	lm.Lock()
	// check if entry does not exists, then add entry
	if _, exists := lm.mutexMap[entry]; !exists {
		lm.addEntry(entry)
	}

	lm.Unlock()
	lm.lockEntry(entry)
}

// UnlockEntry release the lock associated with the specific entry
func (lm *LockMap) UnlockEntry(entry string) {
	lm.Lock()
	defer lm.Unlock()

	if _, exists := lm.mutexMap[entry]; !exists {
		return
	}
	lm.unlockEntry(entry)
}

func (lm *LockMap) addEntry(entry string) {
	lm.mutexMap[entry] = &sync.Mutex{}
}

func (lm *LockMap) lockEntry(entry string) {
	lm.mutexMap[entry].Lock()
}

func (lm *LockMap) unlockEntry(entry string) {
	lm.mutexMap[entry].Unlock()
}

func ConvertTagsToMap(tags string) (map[string]string, error) {
	m := make(map[string]string)
	if tags == "" {
		return m, nil
	}
	s := strings.Split(tags, tagsDelimiter)
	for _, tag := range s {
		kv := strings.Split(tag, tagKeyValueDelimiter)
		if len(kv) != 2 {
			return nil, fmt.Errorf("Tags '%s' are invalid, the format should like: 'key1=value1,key2=value2'", tags)
		}
		key := strings.TrimSpace(kv[0])
		if key == "" {
			return nil, fmt.Errorf("Tags '%s' are invalid, the format should like: 'key1=value1,key2=value2'", tags)
		}
		value := strings.TrimSpace(kv[1])
		m[key] = value
	}
	return m, nil
}

type OsInfo struct {
	Distro  string
	Version string
}

const (
	keyDistribID      = "DISTRIB_ID"
	keyDistribRelease = "DISTRIB_RELEASE"
)

func GetOSInfo(f interface{}) (*OsInfo, error) {
	cfg, err := ini.Load(f)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %q", f)
	}

	oi := &OsInfo{}
	oi.Distro = cfg.Section("").Key(keyDistribID).String()
	oi.Version = cfg.Section("").Key(keyDistribRelease).String()

	klog.V(2).Infof("get OS info: %v", oi)
	return oi, nil
}
