package router
import (
	"fmt"
	"net"
	"net/url"
)


func router() {
//acc://root_name[/sub-chain name[/sub-chain]...]
//acc://a:Big.Company/atk
	//s := "postgres://user:pass@host.com:5432/path?k=v#f"
	s := "acc://a:Big.Company/atk"

	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}

	fmt.Println(u.Scheme)

	fmt.Println(u.User)
	fmt.Println(u.User.Username())
	p, _ := u.User.Password()
	fmt.Println(p)

	fmt.Println(u.Host)
	host, port, _ := net.SplitHostPort(u.Host)
	fmt.Println(host)
	fmt.Println(port)

	fmt.Println(u.Path)
	fmt.Println(u.Fragment)

	fmt.Println(u.RawQuery)
	m, _ := url.ParseQuery(u.RawQuery)
	fmt.Println(m)
	fmt.Println(m["k"][0])
}

