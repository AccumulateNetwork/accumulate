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
      Name: "Acadia",
      Port: 26500,
      Ip:  []string{
          "192.168.1.171",
          "192.168.1.68",
          "192.168.1.119",
          "192.168.1.74",
      },
    },
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
    RouterNode {
      Name: "Badlands",
      Port: 26530,
      Ip:  []string{
          "192.168.4.171",
          "192.168.4.68",
          "192.168.4.119",
          "192.168.4.74",
      },
    },
    RouterNode {
      Name: "Big-Bend",
      Port: 26540,
      Ip:  []string{
          "192.168.5.171",
          "192.168.5.68",
          "192.168.5.119",
          "192.168.5.74",
      },
    },
    RouterNode {
      Name: "Biscayne",
      Port: 26550,
      Ip:  []string{
          "192.168.6.171",
          "192.168.6.68",
          "192.168.6.119",
          "192.168.6.74",
      },
    },
    RouterNode {
      Name: "Black-Canyon-of-the-Gunnison",
      Port: 26560,
      Ip:  []string{
          "192.168.7.171",
          "192.168.7.68",
          "192.168.7.119",
          "192.168.7.74",
      },
    },
    RouterNode {
      Name: "Bryce-Canyon",
      Port: 26570,
      Ip:  []string{
          "192.168.8.171",
          "192.168.8.68",
          "192.168.8.119",
          "192.168.8.74",
      },
    },
    RouterNode {
      Name: "Canyonlands",
      Port: 26580,
      Ip:  []string{
          "192.168.9.171",
          "192.168.9.68",
          "192.168.9.119",
          "192.168.9.74",
      },
    },
    RouterNode {
      Name: "Capitol-Reef",
      Port: 26590,
      Ip:  []string{
          "192.168.10.171",
          "192.168.10.68",
          "192.168.10.119",
          "192.168.10.74",
      },
    },
    RouterNode {
      Name: "Carlsbad-Caverns",
      Port: 26600,
      Ip:  []string{
          "192.168.11.171",
          "192.168.11.68",
          "192.168.11.119",
          "192.168.11.74",
      },
    },
    RouterNode {
      Name: "Channel-Islands",
      Port: 26610,
      Ip: []string{
          "192.168.12.171",
          "192.168.12.68",
          "192.168.12.119",
          "192.168.12.74",
      },
    },
    RouterNode {
      Name: "Congaree",
      Port: 26620,
      Ip:  []string{
          "192.168.13.171",
          "192.168.13.68",
          "192.168.13.119",
          "192.168.13.74",
      },
    },
    RouterNode {
      Name: "Crater-Lake",
      Port: 26630,
      Ip:  []string{
          "192.168.14.171",
          "192.168.14.68",
          "192.168.14.119",
          "192.168.14.74",
      },
    },
    RouterNode {
      Name: "Cuyahoga-Valley",
      Port: 26640,
      Ip:  []string{
          "192.168.15.171",
          "192.168.15.68",
          "192.168.15.119",
          "192.168.15.74",
      },
    },
    RouterNode {
      Name: "Death-Valley",
      Port: 26650,
      Ip:  []string{
          "192.168.16.171",
          "192.168.16.68",
          "192.168.16.119",
          "192.168.16.74",
      },
    },
    RouterNode {
      Name: "Denali",
      Port: 26660,
      Ip:  []string{
          "192.168.17.171",
          "192.168.17.68",
          "192.168.17.119",
          "192.168.17.74",
      },
    },
    RouterNode {
      Name: "Dry-Tortugas",
      Port: 26670,
      Ip:  []string{
          "192.168.18.171",
          "192.168.18.68",
          "192.168.18.119",
          "192.168.18.74",
      },
    },
    RouterNode {
      Name: "Everglages",
      Port: 26680,
      Ip:  []string{
          "192.168.19.171",
          "192.168.19.68",
          "192.168.19.119",
          "192.168.19.74",
      },
    },
    RouterNode {
      Name: "Gates-of-the-Artic",
      Port: 26690,
      Ip:  []string{
          "192.168.20.171",
          "192.168.20.68",
          "192.168.20.119",
          "192.168.20.74",
      },
    },
    RouterNode {
      Name: "Gateway-Arch",
      Port: 26700,
      Ip:  []string{
          "192.168.21.171",
          "192.168.21.68",
          "192.168.21.119",
          "192.168.21.74",
      },
    },
    RouterNode {
      Name: "Glacier",
      Port: 26710,
      Ip:  []string{
          "192.168.22.171",
          "192.168.22.68",
          "192.168.22.119",
          "192.168.22.74",
      },
    },
    RouterNode {
      Name: "Glacier-Bay",
      Port: 26720,
      Ip:  []string{
          "192.168.23.171",
          "192.168.23.68",
          "192.168.23.119",
          "192.168.23.74",
      },
    },
    RouterNode {
      Name: "Grand-Canyon",
      Port: 26730,
      Ip:  []string{
          "192.168.24.171",
          "192.168.24.68",
          "192.168.24.119",
          "192.168.24.74",
      },
    },
    RouterNode {
      Name: "Grand-Teton",
      Port: 26740,
      Ip:  []string{
          "192.168.25.171",
          "192.168.25.68",
          "192.168.25.119",
          "192.168.25.74",
      },
    },
    RouterNode {
      Name: "Great-Basin",
      Port: 26750,
      Ip:  []string{
          "192.168.26.171",
          "192.168.26.68",
          "192.168.26.119",
          "192.168.26.74",
      },
    },
    RouterNode {
      Name: "Great-Sand-Dunes",
      Port: 26760,
      Ip:  []string{
          "192.168.27.171",
          "192.168.27.68",
          "192.168.27.119",
          "192.168.27.74",
      },
    },
    RouterNode {
      Name: "Great-Smoky-Mountains",
      Port: 26770,
      Ip:  []string{
          "192.168.28.171",
          "192.168.28.68",
          "192.168.28.119",
          "192.168.28.74",
      },
    },
    RouterNode {
      Name: "Guadalupe-Mountains",
      Port: 26780,
      Ip:  []string{
          "192.168.29.171",
          "192.168.29.68",
          "192.168.29.119",
          "192.168.29.74",
      },
    },
    RouterNode {
      Name: "Haleakala",
      Port: 26790,
      Ip:  []string{
          "192.168.30.171",
          "192.168.30.68",
          "192.168.30.119",
          "192.168.30.74",
      },
    },
    RouterNode {
      Name: "Hawaii-Volcanoes",
      Port: 26800,
      Ip:  []string{
          "192.168.31.171",
          "192.168.31.68",
          "192.168.31.119",
          "192.168.31.74",
      },
    },
    RouterNode {
      Name: "Hot-Springs",
      Port: 26810,
      Ip:  []string{
          "192.168.32.171",
          "192.168.32.68",
          "192.168.32.119",
          "192.168.32.74",
      },
    },
    RouterNode {
      Name: "Indiana-Dunes",
      Port: 26820,
      Ip:  []string{
          "192.168.33.171",
          "192.168.33.68",
          "192.168.33.119",
          "192.168.33.74",
      },
    },
    RouterNode {
      Name: "Isle-Royale",
      Port: 26830,
      Ip:  []string{
          "192.168.34.171",
          "192.168.34.68",
          "192.168.34.119",
          "192.168.34.74",
      },
    },
    RouterNode {
      Name: "Joshua-Tree",
      Port: 26840,
      Ip:  []string{
          "192.168.35.171",
          "192.168.35.68",
          "192.168.35.119",
          "192.168.35.74",
      },
    },
    RouterNode {
      Name: "Katmai",
      Port: 26850,
      Ip:  []string{
          "192.168.36.171",
          "192.168.36.68",
          "192.168.36.119",
          "192.168.36.74",
      },
    },
    RouterNode {
      Name: "Kenai-Fjords",
      Port: 26860,
      Ip:  []string{
          "192.168.37.171",
          "192.168.37.68",
          "192.168.37.119",
          "192.168.37.74",
      },
    },
    RouterNode {
      Name: "Kings-Canyon",
      Port: 26870,
      Ip:  []string{
          "192.168.38.171",
          "192.168.38.68",
          "192.168.38.119",
          "192.168.38.74",
      },
    },
    RouterNode {
      Name: "Kobuk-Valley",
      Port: 26880,
      Ip:  []string{
          "192.168.39.171",
          "192.168.39.68",
          "192.168.39.119",
          "192.168.39.74",
      },
    },
    RouterNode {
      Name: "Lake-Clark",
      Port: 26890,
      Ip:  []string{
          "192.168.40.171",
          "192.168.40.68",
          "192.168.40.119",
          "192.168.40.74",
      },
    },
    RouterNode {
      Name: "Lassen-Volcanic",
      Port: 26900,
      Ip:  []string{
          "192.168.41.171",
          "192.168.41.68",
          "192.168.41.119",
          "192.168.41.74",
      },
    },
    RouterNode {
      Name: "Mammoth-Cave",
      Port: 26910,
      Ip:  []string{
          "192.168.42.171",
          "192.168.42.68",
          "192.168.42.119",
          "192.168.42.74",
      },
    },
    RouterNode {
      Name: "Mesa-Verde",
      Port: 26920,
      Ip:  []string{
          "192.168.43.171",
          "192.168.43.68",
          "192.168.43.119",
          "192.168.43.74",
      },
    },
    RouterNode {
      Name: "Mount-Rainier",
      Port: 26930,
      Ip:  []string{
          "192.168.44.171",
          "192.168.44.68",
          "192.168.44.119",
          "192.168.44.74",
      },
    },
    RouterNode {
      Name: "New-River-Gorge",
      Port: 26940,
      Ip:  []string{
          "192.168.45.171",
          "192.168.45.68",
          "192.168.45.119",
          "192.168.45.74",
      },
    },
    RouterNode {
      Name: "North-Cascades",
      Port: 26950,
      Ip:  []string{
          "192.168.46.171",
          "192.168.46.68",
          "192.168.46.119",
          "192.168.46.74",
      },
    },
    RouterNode {
      Name: "Olympic",
      Port: 26960,
      Ip:  []string{
          "192.168.47.171",
          "192.168.47.68",
          "192.168.47.119",
          "192.168.47.74",
      },
    },
    RouterNode {
      Name: "Pertrified-Forest",
      Port: 26970,
      Ip:  []string{
          "192.168.48.171",
          "192.168.48.68",
          "192.168.48.119",
          "192.168.48.74",
      },
    },
    RouterNode {
      Name: "Pinnacles",
      Port: 26980,
      Ip:  []string{
          "192.168.49.171",
          "192.168.49.68",
          "192.168.49.119",
          "192.168.49.74",
      },
    },
    RouterNode {
      Name: "Redwood",
      Port: 26990,
      Ip:  []string{
          "192.168.50.171",
          "192.168.50.68",
          "192.168.50.119",
          "192.168.50.74",
      },
    },
    RouterNode {
      Name: "Rocky-Mountain",
      Port: 27000,
      Ip:  []string{
          "192.168.51.171",
          "192.168.51.68",
          "192.168.51.119",
          "192.168.51.74",
      },
    },
    RouterNode {
      Name: "Saguaro",
      Port: 27010,
      Ip:  []string{
          "192.168.52.171",
          "192.168.52.68",
          "192.168.52.119",
          "192.168.52.74",
      },
    },
    RouterNode {
      Name: "Sequoia",
      Port: 27020,
      Ip:  []string{
          "192.168.53.171",
          "192.168.53.68",
          "192.168.53.119",
          "192.168.53.74",
      },
    },
    RouterNode {
      Name: "Shenandoah",
      Port: 27030,
      Ip:  []string{
          "192.168.54.171",
          "192.168.54.68",
          "192.168.54.119",
          "192.168.54.74",
      },
    },
    RouterNode {
      Name: "Theodore-Roosevelt",
      Port: 27040,
      Ip:  []string{
          "192.168.55.171",
          "192.168.55.68",
          "192.168.55.119",
          "192.168.55.74",
      },
    },
    RouterNode {
      Name: "Virgin-Islands",
      Port: 27050,
      Ip:  []string{
          "192.168.56.171",
          "192.168.56.68",
          "192.168.56.119",
          "192.168.56.74",
      },
    },
    RouterNode {
      Name: "Voyageurs",
      Port: 27060,
      Ip:  []string{
          "192.168.57.171",
          "192.168.57.68",
          "192.168.57.119",
          "192.168.57.74",
      },
    },
    RouterNode {
      Name: "WhiteSands",
      Port: 27080,
      Ip:  []string{
          "192.168.58.171",
          "192.168.58.68",
          "192.168.58.119",
          "192.168.58.74",
      },
    },
    RouterNode {
      Name: "Wind-Cave",
      Port: 27090,
      Ip:  []string{
          "192.168.59.171",
          "192.168.59.68",
          "192.168.59.119",
          "192.168.59.74",
      },
    },
    RouterNode {
      Name: "Wrangell-Saint-Elias",
      Port: 27100,
      Ip:  []string{
          "192.168.60.171",
          "192.168.60.68",
          "192.168.60.119",
          "192.168.60.74",
      },
    },
    RouterNode {
      Name: "Yellowstone",
      Port: 27110,
      Ip:  []string{
          "192.168.61.171",
          "192.168.61.68",
          "192.168.61.119",
          "192.168.61.74",
      },
    },
    RouterNode {
      Name: "Yosemite",
      Port: 27120,
      Ip:  []string{
          "192.168.62.171",
          "192.168.62.68",
          "192.168.62.119",
          "192.168.62.74",
      },
    },
    RouterNode {
      Name: "Zion",
      Port: 27130,
      Ip:  []string{
          "192.168.63.171",
          "192.168.63.68",
          "192.168.63.119",
          "192.168.63.74",
      },
    },
  }
}
