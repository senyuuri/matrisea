import React, { useState } from 'react';
import { Breadcrumb, Row, Button, } from 'antd';
import QueueAnim from 'rc-queue-anim';

import NewVMForm from './components/NewVMForm';
import DeviceTable from './components/DeviceTable';

const data = [
  {
    key: '1',
    id: '15db08f938a4',
    name: 'matrisea-cvd-JTcFAR',
    device_type: 'cuttlefish-kvm',
    aosp_version: 'aosp_cf_x86_64_phone-img-7530437',
    created: '2020-01-01 00:00:00',
    status: 'Running',
    tags: ["Android 11",]
  },
  {
    key: '1',
    id: '15db08f938a4',
    name: 'matrisea-cvd-JTcFAR',
    device_type: 'cuttlefish-kvm',
    aosp_version: 'aosp_cf_x86_64_phone-img-7530437',
    created: '2020-01-01 00:00:00',
    status: 'Running',
    tags: ["Android 11", "Custom Kernel"]
  },
];

function DeviceList(){
  const [formVisible, setFormVisible] = useState(false);
  
  function handleFormClose() {
    setFormVisible(false);
  }

  return (
    <div key="device-list">
        <div className="site-layout-content">
          <QueueAnim key="content" type={['right', 'left']}>
            <Row justify="space-between" key="1">
              <Breadcrumb>
                <Breadcrumb.Item>Home</Breadcrumb.Item>
                <Breadcrumb.Item>Devices</Breadcrumb.Item>
              </Breadcrumb>
              <Button onClick={() => {setFormVisible(true);}}>New Virtual Device</Button>
            </Row>
            <DeviceTable data={data} key="2"/>
          </QueueAnim>
        </div>
        <NewVMForm visible={formVisible} onChange={handleFormClose}/>
  </div>
  )
}

export default DeviceList;