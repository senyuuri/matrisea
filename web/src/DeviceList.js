import React, { useState, useEffect, useCallback } from 'react';
import { Breadcrumb, Row, Button, message} from 'antd';
import { WsContext } from './Context';

import NewVMForm from './components/NewVMForm';
import DeviceTable from './components/DeviceTable';

function DeviceList(){
  const ws = React.useContext(WsContext);

  const [formVisible, setFormVisible] = useState(false);
  const [deviceList, setDeviceList] = useState([]);

  function handleFormClose() {
    setFormVisible(false);
  }

  const requestDeviceListUpdate = useCallback(()=> {
    if (ws && ws.readyState === 1) {
      ws.send(JSON.stringify({
        type: 0
      }));
    }
  },[ws]);

  function handleDeviceListUpdate(e) {
    var msg = JSON.parse(e.data);
    // type 0: WS_TYPE_LIST_VM
    if (msg.type === 0){
      if(msg.has_error) {
        message.error('Unable to get VM status due to', msg.error)  
      }
      else {
        msg.data.vms.forEach((vm) => {
          vm['id'] = vm['id'].substring(0,10);
          vm['key'] = vm['id'];
          let date = new Date(vm['created']*1000);
          vm['created'] = date.toLocaleString('en-US', { timeZone: 'Asia/Singapore' });
        });
        setDeviceList(msg.data.vms);
      }
    }
  };

  useEffect(() => {
    if(ws){
      requestDeviceListUpdate()
      const interval = setInterval(() =>{
        requestDeviceListUpdate();
      }, 5000);
      
      ws.addEventListener("open", requestDeviceListUpdate)
      ws.addEventListener("message", handleDeviceListUpdate);
      return () => {
        ws.removeEventListener("open", requestDeviceListUpdate);
        ws.removeEventListener("message", handleDeviceListUpdate);
        clearInterval(interval);
      }
    }
  },[ws, requestDeviceListUpdate]);

  return (
    <div key="device-list">
        <div className="site-layout-content">
          <Row justify="space-between" key="1">
            <Breadcrumb>
              <Breadcrumb.Item>Home</Breadcrumb.Item>
              <Breadcrumb.Item>Devices</Breadcrumb.Item>
            </Breadcrumb>
            <Button onClick={() => {setFormVisible(true);}}>Create Virtual Device</Button>
          </Row>
          <DeviceTable data={deviceList} key="2"/>
        </div>
        <NewVMForm visible={formVisible} onChange={handleFormClose}/>
  </div>
  )
}

export default DeviceList;