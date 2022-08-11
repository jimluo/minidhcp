package base

import (
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/spf13/viper"
)

var log = GetLogger("config")

type (
	Subnet struct {
		Role      string `yaml:"role"`
		IpStart   string `yaml:"ipstart"`
		IpStop    string `yaml:"ipstop"`
		Dns       string `yaml:"dns"`
		Router    string `yaml:"router"`
		Netmask   string `yaml:"netmask"`
		LeaseTime string `yaml:"leasetime"`
	}
	Config struct {
		RestPort      string `yaml:"restport"`
		Ifname        string `yaml:"ifname"`
		ServerId      string `yaml:"serverid"`
		Guest         Subnet `yaml:"guest"`
		Staff         Subnet `yaml:"staff"`
		Boss          Subnet `yaml:"boss"`
		Range1        Subnet `yaml:"range1"`
		Staticrouter1 string `yaml:"staticrouter1"`
	}
)

// Read the config file from the current directory and marshal into the conf config struct.
func LoadConfig() *Config {
	viper.AddConfigPath(".")
	viper.SetConfigName("minidhcp")
	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("%v", err)
	}

	conf := &Config{}
	err = viper.Unmarshal(conf)
	if err != nil {
		log.Printf("unable to decode into config struct, %v", err)
	}

	log.Printf("conf: %v", conf)
	return conf
}

func (c *Config) Address() net.UDPAddr {
	return net.UDPAddr{
		IP:   net.IPv4(0, 0, 0, 0),
		Port: dhcpv4.ServerPort,
		Zone: c.Ifname,
	}
}

func (c *Config) Marshal() {
	if err := viper.SafeWriteConfig(); err != nil {
		log.Errorf("Config write error: ", err)
	}
}
