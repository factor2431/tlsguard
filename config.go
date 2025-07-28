package main

type Config struct {
	ID                       string            `yaml:"id"`
	MTU                      int               `yaml:"mtu"`
	Name                     string            `yaml:"name"`
	Mode                     string            `yaml:"mode"`
	IPv4                     []string          `yaml:"ipv4"`
	IPv6                     []string          `yaml:"ipv6"`
	Threads                  int               `yaml:"threads"`
	Endpoint                 string            `yaml:"endpoint"`
	Insecure                 bool              `yaml:"insecure"`
	CABundle                 string            `yaml:"ca-bundle"`
	ListenAddr               string            `yaml:"listen-addr"`
	ListenPort               int               `yaml:"listen-port"`
	Certificate              ConfigCertificate `yaml:"certificate"`
	ConnectionCount          int               `yaml:"connection-count"`
	DeviceToRemoteBufferSize int               `yaml:"device-to-remote-buffer-size"`
	RemoteToDeviceBufferSize int               `yaml:"remote-to-device-buffer-size"`
}

type ConfigCertificate struct {
	Crt string `yaml:"crt"`
	Key string `yaml:"key"`
}
