import React from 'react';
import { Menu, Breadcrumb, Row, Col, Button, PageHeader, Image, Spin } from 'antd';
import { PoweroffOutlined, SettingOutlined, InteractionOutlined, BarsOutlined } from '@ant-design/icons';
import VncDisplay from 'react-vnc-display';
import QueueAnim from 'rc-queue-anim';
import WebTerminal from './components/Terminal';

function DeviceDetail(){
  const MyPageHeader = React.forwardRef((props, ref) => (
    <PageHeader
      innerRef={ref}
      key="2"
      ghost={false}
      onBack={() => window.history.back()}
      title="matrisea-aaa-bbb"
      subTitle="aosp-aaaaaaaaaaaa / custom-kernel"
      extra={[
        <Button icon={<PoweroffOutlined />} key="power-btn">Power</Button>
      ]}
      {...props}
    />
  ));
  
  return (
    <div key="device-detail">
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
          <Row gutter={16}  key="3">
              <Col span={6}>
                <VncDisplay url="ws://localhost:6080/"/>
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
                <WebTerminal/>
              </Col>
            </Row>
        </QueueAnim>
      </div>
    </div>
  )
}

export default DeviceDetail;