POST http://127.0.0.1:36062/dhcp/range
{
    "rdata":[
        {
            "name": "staff",
            "vlan": "",
            "ipstart": "10.10.10.100",
            "ipstop": "10.10.10.200",
            "leasetime": "60s",
            "mask": "255.255.255.0",
            "gateway": "192.168.1.1",
            "dns1": "192.168.1.100"
        },
        {
            "name": "guest",
            "vlan": "",
            "ipstart": "192.168.3.1",
            "ipstop": "192.168.3.252",
            "leasetime": "60s",
            "mask": "255.255.255.0",
            "gateway": "192.168.1.1",
            "dns1": "192.168.1.100"
        }        
    ]
}

HTTP/1.1 200
"{\"rcode\":\"QS000000\",\"rmsg\":\"success\",\"rdata\":\"success\"}"