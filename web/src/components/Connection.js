import React, { useState, useEffect, useCallback } from 'react';
import { Row, Col, Badge, Card, Radio, Input, Typography, message} from 'antd';
import { AndroidOutlined, CloudOutlined } from '@ant-design/icons';
import axios from 'axios';

const { Text } = Typography;
const { TextArea } = Input;

function Connection(props){
  const API_ENDPOINT = window.location.protocol+ "//"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1"
	const [mode, setMode] = useState("direct");
  const [lanIPs, setLANIPs] = useState(["{lan_ip}"]);
  const [tailscaleIP, setTailscaleIP] = useState("{tailscale_ip}");
  const adbPort = 6520 + parseInt(props.cf_instance) -1;

	const onSelectConnectionMode = (e) => {
		setMode(e.target.value);
	};

  const getConnectionIPs = useCallback(() => {
    var url = API_ENDPOINT + '/ips';
    axios.get(url)
    .then(function (response) {
      let data = response.data;
      setLANIPs(data.lan_ips);
      setTailscaleIP(data.tailscale_ip);
    })
    .catch(function (error) {
      message.error("Failed to get connection IPs due to " + error)
    })
  }, [API_ENDPOINT]);

  useEffect(() => {
    getConnectionIPs()
  }, [getConnectionIPs])

	return (
		<div id="menu-content-connection">
			<Row>
				<Card title={<div> <CloudOutlined /> Select Connection Mode</div>}  bordered={false} style={{width: "100%"}}>
				  <Text strong>Direct: </Text><Text>Connect from the same subnet of the server (e.g. for self-hosted Matrisea)</Text><br/>
				  <Text strong>Tailscale: </Text><Text>Connect from anywhere on the Internet using Tailscale point-to-point VPN and NAT relays</Text><br/>
				  <br/>
          <Text> Connection Mode:  </Text>
          <Radio.Group onChange={onSelectConnectionMode} value={mode}>
            <Radio.Button value="direct">Direct</Radio.Button>
            <Radio.Button value="tailscale">Tailscale</Radio.Button>
          </Radio.Group>
				</Card>
			  </Row>

        <div style={{display: mode==="direct" ? 'block' : 'none'}}>
			    <Row>
            <Col span={10}  >
              <Badge.Ribbon text="Direct" color="green">
              <Card title={<div> <CloudOutlined /> Step 1: Get LAN Access</div>}  bordered={false} style={{height: "100%"}}>
                <Text strong> Server LAN IPs:</Text><br/>
                {
                  lanIPs.map((ip) => (
                    <Text code key={ip}>{ip} </Text>
                  ))
                }
                <br/><br/>
                <Text strong>Requirements:</Text>
                <ul>
                  <li>Make sure you're connected to the same subnet as of the server.</li>
                  <li>You will need a valid local account on the remote server that allows you to do SSH port forwarding.</li>
                </ul>
              </Card>
              </Badge.Ribbon>
            </Col>
            <Col span={14}>
              <Badge.Ribbon text="Direct" color="green">
              <Card title={<div> <AndroidOutlined /> Step 2: Connect to Device</div>} bordered={false} style={{height: "100%"}}>
                <div className="connection-row">
                  <p>SSH into workspace</p>
                  <Text code>ssh -J [username]@{lanIPs} vsoc-01@{props.deviceDetail['ip']}</Text>
                </div>
                <div className="connection-row">
                  <p>ADB connect to the device (via SSH port forwarding)</p>
                  <Text code># open two separate terminals and enter</Text><br/>
                  <Text code>ssh -L 9999:localhost:{adbPort} -N [username]@{lanIPs}</Text><br/>
                  <Text code>adb connect localhost:9999</Text>
                </div>
              </Card>
              </Badge.Ribbon>
            </Col>
          </Row>
        </div>

        <div style={{display: mode==="tailscale" ? 'block' : 'none'}}>
          <Row>
            <Col span={10}  >
              <Badge.Ribbon text="Tailscale" color="blue">
              <Card title={<div> <CloudOutlined /> Step 1: Get LAN Access</div>}  bordered={false} style={{height: "100%"}}>
                <Text strong> Server Tailscale IP:</Text><br/> 
                <Text code>{tailscaleIP}</Text> <br/><br/>
                <Text strong>Setup Tailscale:</Text>
                <ol>
                  <li>Download and Install Tailscale client (https://tailscale.com/download/)</li>
                  <li>Connect to the control server</li>
                  <TextArea spellcheck="false" autoSize={true} value="tailscale up --login-server https://aaaaaaa.com/tailscale --authkey a8ce88557831d6267acfeb70a7ff685f"/>
                  <li>You will need a valid local account on the remote server that allows you to do SSH port forwarding</li>
                </ol>
              </Card>
              </Badge.Ribbon>
            </Col>
            <Col span={14}>
              <Badge.Ribbon text="Tailscale" color="blue">
              <Card title={<div> <AndroidOutlined /> Step 2: Connect to Device</div>} bordered={false} style={{height: "100%"}}>
                <div className="connection-row">
                  <p>SSH into workspace</p>
                  <Text code>ssh -J [username]@{tailscaleIP} vsoc-01@{props.deviceDetail['ip']}</Text>
                </div>
                <div className="connection-row">
                <p>ADB connect to the device (via SSH port forwarding)</p>
                  <Text code># open two separate terminals and enter</Text><br/>
                  <Text code>ssh -L 9999:localhost:{adbPort} -N [username]@{tailscaleIP}</Text><br/>
                  <Text code>adb connect localhost:9999</Text>
                </div>
              </Card>
              </Badge.Ribbon>
            </Col>
			    </Row>
        </div>
		</div>
	)

}

export default Connection;