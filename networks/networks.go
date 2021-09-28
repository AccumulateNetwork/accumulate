package networks

type RouterNode struct {
	Name string
	Port int
	Ip   []string
}

var Networks []RouterNode

func init() {
	Networks = []RouterNode{
		{
			Name: "Arches",
			Port: 33000,
			Ip: []string{
				"13.51.10.110",
				"13.232.230.216",
			},
		},
		{
			Name: "AmericanSamoa",
			Port: 33000,
			Ip: []string{
				"18.221.39.36",
				"44.236.45.58",
			},
		},
		{
			Name: "Badlands",
			Port: 35550,
			Ip: []string{
				"127.0.0.1",
			},
		},
		{
			Name: "EastXeons",
			Port: 33000,
			Ip: []string{
				"18.119.26.7",
				"18.119.149.208",
			},
		},
		{
			Name: "Localhost",
			Port: 26656,
			Ip: []string{
				"127.0.1.1",
				"127.0.1.2",
				"127.0.1.3",
			},
		},
	}
}

func IndexOf(name string) int {
	for i, net := range Networks {
		if net.Name == name {
			return i
		}
	}
	return -1
}
