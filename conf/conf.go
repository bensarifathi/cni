package conf

import (
	"encoding/json"
	"log"
)

type NetConfig struct {
	CniVersion string `json:"CniVersion"`
	Name       string `json:"name"`
	Check      bool   `json:"disableCheck"`
	// Plugins    []Plugin `json:"plugins"`
	Plugin
}

type Plugin struct {
	Type    string `json:"myBridge"`
	Bridge  string `json:"bridge"`
	PodCIDR string `json:"podCIDR"`
	Subnet  string `json:"subnet"`
	Gateway string `json:"gateway"`
	Route   string `json:"route"`
}

func LoadNetConf(confFile []byte) *NetConfig {
	conf := &NetConfig{}
	if err := json.Unmarshal(confFile, conf); err != nil {
		log.Fatalf("Error while unmarshaling config data: %s", err.Error())
	}
	return conf
}

func (nconf *NetConfig) String() string {
	nformat, _ := json.MarshalIndent(nconf, "", "	")
	return string(nformat)
}
