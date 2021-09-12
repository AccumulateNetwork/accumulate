package router

type RouterNode struct {
    Name string
    Port int
    Ip []string
}

var Networks []RouterNode

func init() {
  Networks = []RouterNode{
    RouterNode {
      Name: "Arches",
      Port: 33000,
      Ip:  []string{
          "13.51.10.110",
          "13.232.230.216",
      },
    },
    RouterNode {
      Name: "AmericanSamoa",
      Port: 33000,
      Ip:  []string{
          "18.221.39.36",
          "44.236.45.58",
      },
    },
  }
}
