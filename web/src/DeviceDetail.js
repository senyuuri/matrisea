import React, { useEffect, useState } from 'react';
import { useParams } from "react-router-dom";
import { Menu, Breadcrumb, Row, Col, Button, PageHeader, message} from 'antd';
import { PoweroffOutlined, SettingOutlined, InteractionOutlined, BarsOutlined } from '@ant-design/icons';
import QueueAnim from 'rc-queue-anim';
import WebTerminal from './components/Terminal';
import VNCDisplay from './components/VNCDisplay';
import axios from 'axios';

function DeviceDetail(){
  const { device_name, cf_instance } = useParams();
  const API_ENDPOINT = window.location.protocol+ "//"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1"
  const VNC_WS_URL = "ws://"+  window.location.hostname + ":" + (parseInt(process.env.REACT_APP_VNC_PORT) + parseInt(cf_instance)-1);
  const [deviceDetail, setDeviceDetail] = useState({});
  const [deviceDescription, setDeviceDescription] = useState("");
  

  const MyPageHeader = React.forwardRef((props, ref) => (
    <PageHeader
      innerRef={ref}
      key="2"
      ghost={false}
      onBack={() => window.history.back()}
      title={device_name}
      subTitle={deviceDescription}
      extra={<>
        <Button icon={<PoweroffOutlined />} key="install-btn">Install APK</Button>
        <Button icon={<PoweroffOutlined />} key="power-btn">Power</Button>
      </>}
      {...props}
    />
  ));

  useEffect(() => {
    var url = API_ENDPOINT + '/vms/' + device_name
    axios.get(url)
    .then(function (response) {
      setDeviceDetail(response.data);
    })
    .catch(function (error) {
      if (error.response) {
        message.error("Failed to get device " + device_name + "status due to " + error.response.status + " - " + error.response.data['error']);
      }
    })
  }, [API_ENDPOINT, device_name])

  useEffect(() => {
    if(Object.keys(deviceDetail).length != 0){
      const items = [
        deviceDetail['cpu'] + " vCPU",
        deviceDetail['ram'] + " GB RAM",
        deviceDetail['ip'],
        "Container ID" + " " +deviceDetail['id'].slice(0,8),
      ]
      setDeviceDescription(items.join(" / "))
    }
  }, [deviceDetail])
  
  return (
    <div className="site-layout-content">
      <QueueAnim key="content" type={['right', 'left']}>
        <Row justify="space-between" key="1">
          <Breadcrumb>
            <Breadcrumb.Item>Home</Breadcrumb.Item>
            <Breadcrumb.Item>Device</Breadcrumb.Item>
            <Breadcrumb.Item>{device_name}</Breadcrumb.Item>
          </Breadcrumb>
        </Row>
        <MyPageHeader/>
        <Row gutter={16}  key="3" id="detail-flex-content">
          <Col span={6}>
            <VNCDisplay url={VNC_WS_URL}/>
            {/* <Spin spinning={true} tip="Waiting for device...">
              </Spin> */}
          </Col>
          <Col span={18}>
            <Menu mode="horizontal" selectedKeys="terminal">
              <Menu.Item key="terminal" icon={<InteractionOutlined />}>
                Terminal
              </Menu.Item>
              <Menu.Item key="log" icon={<BarsOutlined />}>
                Device Log
              </Menu.Item>
              <Menu.Item key="settings" icon={<SettingOutlined />}>
                Settings
              </Menu.Item>
            </Menu>
            <WebTerminal deviceName={device_name}/>
          </Col>
        </Row>
      </QueueAnim>
    </div>
  )
}

export default DeviceDetail;