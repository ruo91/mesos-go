package detector

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"net"
	"strconv"
	"strings"
	"sync"

	log "github.com/golang/glog"
	mesos "github.com/mesos/mesos-go/mesosproto"
	util "github.com/mesos/mesos-go/mesosutil"
	"github.com/mesos/mesos-go/upid"
)

var (
	pluginLock sync.Mutex
	plugins    = map[string]PluginFactory{}
)

type PluginFactory func(string) (Master, error)

// associates a plugin implementation with a Master specification prefix.
// packages that provide plugins are expected to invoke this func within
// their init() implementation. schedulers that wish to support plugins may
// anonymously import ("_") a package the auto-registers said plugins.
func Register(prefix string, f PluginFactory) error {
	if prefix == "" {
		return fmt.Errorf("illegal prefix: '%v'", prefix)
	}
	if f == nil {
		return fmt.Errorf("nil plugin factories are not allowed")
	}

	pluginLock.Lock()
	defer pluginLock.Unlock()

	if _, found := plugins[prefix]; found {
		return fmt.Errorf("detection plugin already registered for prefix '%s'", prefix)
	}
	plugins[prefix] = f
	return nil
}

func New(spec string) (m Master, err error) {
	if spec == "" {
		m = NewStandalone(nil)
	} else if strings.HasPrefix(spec, "file://") {
		var body []byte
		path := spec[7:]
		body, err = ioutil.ReadFile(path)
		if err != nil {
			log.V(1).Infof("failed to read from file at '%s'", path)
		} else {
			m, err = New(string(body))
		}
	} else if f, ok := MatchingPlugin(spec); ok {
		m, err = f(spec)
	} else if strings.HasPrefix("master@", spec) {
		var pid *upid.UPID
		if pid, err = upid.Parse(spec); err == nil {
			m = NewStandalone(createMasterInfo(pid))
		}
	} else {
		var pid *upid.UPID
		if pid, err = upid.Parse("master@" + spec); err == nil {
			m = NewStandalone(createMasterInfo(pid))
		}
	}
	return
}

func MatchingPlugin(spec string) (PluginFactory, bool) {
	pluginLock.Lock()
	defer pluginLock.Unlock()

	for prefix, f := range plugins {
		if strings.HasPrefix(spec, prefix) {
			return f, true
		}
	}
	return nil, false
}

func createMasterInfo(pid *upid.UPID) *mesos.MasterInfo {
	port, err := strconv.Atoi(pid.Port)
	if err != nil {
		log.Errorf("failed to parse port: %v", err)
		return nil
	}
	ip := net.ParseIP(pid.Host)
	if ip == nil {
		log.Errorf("failed to parse IP address: '%v'", pid.Host)
		return nil
	}
	ip = ip.To4()
	if ip == nil {
		log.Errorf("IP address is not IPv4: %v", pid.Host)
		return nil
	}
	packedip := binary.BigEndian.Uint32(ip) // network byte order is big-endian
	return util.NewMasterInfo(pid.ID, packedip, uint32(port))
}
