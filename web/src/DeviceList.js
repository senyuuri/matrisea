import React, { useState, useEffect } from 'react';
import { Breadcrumb, Row, Button, } from 'antd';
import QueueAnim from 'rc-queue-anim';
import axios from "axios"

import NewVMForm from './components/NewVMForm';
import DeviceTable from './components/DeviceTable';

function DeviceList(){
  const [formVisible, setFormVisible] = useState(false);
  const [deviceList, setDeviceList] = useState([]);

  function handleFormClose() {
    setFormVisible(false);
  }

  useEffect(() => {
    axios.get("http://localhost:8080/api/v1/vms/")
      .then((response) => {
        response.data.forEach((device) => {
          device['id'] = device['id'].substring(0,10);
          device['key'] = device['id'];
          let date = new Date(device['created']*1000);
          device['created'] = date.toLocaleString('en-US', { timeZone: 'Asia/Singapore' });
        })

        setDeviceList(response.data)
      })
      .catch((error) => console.log(error))
  }, []);

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
            <DeviceTable data={deviceList} key="2"/>
          </QueueAnim>
        </div>
        <NewVMForm visible={formVisible} onChange={handleFormClose}/>
  </div>
  )
}

export default DeviceList;