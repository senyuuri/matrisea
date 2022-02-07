import React from 'react';
import { useParams } from "react-router-dom";
import { Menu, Breadcrumb, Row, Col, Button, PageHeader } from 'antd';
import { PoweroffOutlined, SettingOutlined, InteractionOutlined, BarsOutlined } from '@ant-design/icons';
import QueueAnim from 'rc-queue-anim';
import WebTerminal from './components/Terminal';
import VNCDisplay from './components/VNCDisplay';

function DeviceDetail(){
  const { device_name, cf_instance } = useParams();
  const VNC_WS_URL = "ws://"+  window.location.hostname + ":" + (parseInt(process.env.REACT_APP_VNC_PORT) + parseInt(cf_instance)-1);
  console.log(VNC_WS_URL)
  const MyPageHeader = React.forwardRef((props, ref) => (
    <PageHeader
      innerRef={ref}
      key="2"
      ghost={false}
      onBack={() => window.history.back()}
      title={device_name}
      subTitle="aosp-aaaaaaaaaaaa / custom-kernel"
      extra={[
        <Button icon={<PoweroffOutlined />} key="power-btn">Power</Button>
      ]}
      {...props}
    />
  ));
  
  return (
    <div className="site-layout-content">
      <QueueAnim key="content" type={['right', 'left']}>
        <Row justify="space-between" key="1">
          <Breadcrumb>
            <Breadcrumb.Item>Home</Breadcrumb.Item>
            <Breadcrumb.Item>Device</Breadcrumb.Item>
            <Breadcrumb.Item>matrisea-aaa-bbb</Breadcrumb.Item>
          </Breadcrumb>
        </Row>
        <MyPageHeader/>
        <Row gutter={16}  key="3" id="detail-flex-content">
          <Col span={6}>
            <VNCDisplay url={VNC_WS_URL}/>
            {/* <Spin spinning={true} tip="Waiting for device...">
              </Spin> */}
          </Col>
          <Col span={16}>
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