import React, { useEffect, useState, useMemo, useCallback} from 'react';
import { useParams, useHistory } from "react-router-dom";
import { Menu, Breadcrumb, Row, Col, Button, PageHeader, Spin, Image, Badge, message } from 'antd';
import { PoweroffOutlined, SettingOutlined, InteractionOutlined, BarsOutlined, CloudUploadOutlined, LaptopOutlined, FolderOpenOutlined} from '@ant-design/icons';
import QueueAnim from 'rc-queue-anim';
import { LazyLog, ScrollFollow } from 'react-lazylog';
import axios from 'axios';

import WebTerminal from './components/Terminal';
import VNCDisplay from './components/VNCDisplay';
import ApkPickerModal from './components/ApkPickerModal';
import FileExplorer from './components/FileExplorer';
import Connection from './components/Connection';

const { SubMenu } = Menu;
const LOG_SIZE_LIMIT = 1024 * 100;

function DeviceDetail(){
  const history = useHistory();
  const { device_name, cf_instance } = useParams();
  const [deviceDescription, setDeviceDescription] = useState("");
  const [deviceDetail, setDeviceDetail] = useState([]);
  const [installerModalVisible, setInstallerModalVisible] = useState(false);
  const [menuCurrent, setMenuCurrent] = useState("terminal");
  const [logSource, setLogSource] = useState("launcher")
  const [log, setLog] = useState("Waiting for log stream...\n");

  const WS_ENDPOINT = "ws://"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1";
  const API_ENDPOINT = window.location.protocol+ "//"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1"

  // Add logSource to dependencies so to make a new ws connection every time it changes
  // This helps to terminate the old ws whenever the page is closed or the component is dismounted, thus signal to the backend 
  // to always properly stop the go routine for log streaming
  const ws = useMemo(() => new WebSocket(WS_ENDPOINT + "/vms/" + device_name+ "/log/" + logSource), [WS_ENDPOINT, device_name, logSource]);  
  const VNC_WS_URL = "ws://"+  window.location.hostname + ":" + (parseInt(process.env.REACT_APP_VNC_PORT) + parseInt(cf_instance)-1);
  
  const MyPageHeader = React.forwardRef((props, ref) => (
    <PageHeader
      innerRef={ref}
      key="2"
      ghost={false}
      onBack={() => history.push("/")}
      title={device_name}
      subTitle={<div>
          {deviceDescription !== "" && "status" in deviceDetail && deviceDetail["status"] === 0 ?
            <Badge status="default" text="Power Off" style={{paddingRight: "20px"}}/> : ""
          }
          {deviceDescription !== "" && "status" in deviceDetail && deviceDetail["status"] === 1 ?
            <Badge status="success" text="Running" style={{paddingRight: "20px"}}/> : ""
          }
          {deviceDescription !== "" && "status" in deviceDetail && deviceDetail["status"] === 2 ?
            <Badge status="error" text="Error" style={{paddingRight: "20px"}}/> : ""
          }
          {deviceDescription}
        </div> 
      }
      extra={<>
        <Button icon={<CloudUploadOutlined />} key="install-btn" onClick={showInstallerModal}>Install APK</Button>
        {deviceDescription !== "" && "status" in deviceDetail && deviceDetail["status"] === 0 ?
          <Button icon={<PoweroffOutlined />} key="power-btn" onClick={() => startVM(device_name)}>Power</Button> : ""
        }
        {deviceDescription !== "" && "status" in deviceDetail && deviceDetail["status"] === 1 ?
          <Button icon={<PoweroffOutlined />} key="power-btn" className="file-btn-chosen" onClick={() => stopVM(device_name)}>Power</Button> : ""
        }
        {deviceDescription !== "" && "status" in deviceDetail && deviceDetail["status"] === 2 ?
          <Button icon={<PoweroffOutlined />} key="power-btn" disabled={true}>Power</Button> : ""
        }
      </>}
      {...props}
    />
  ));

  const updateDeviceDetail = useCallback(() => {
    var url = API_ENDPOINT + '/vms/' + device_name
    axios.get(url)
    .then(function (response) {
      let deviceDetail = response.data;
      const items = [
        deviceDetail['os_version'],
        deviceDetail['cpu'] + " vCPU",
        deviceDetail['ram'] + " GB RAM",
        deviceDetail['ip'],
        "Container ID " + deviceDetail['id'].slice(0,8)
      ];
      console.log('status', deviceDetail['status'])
      setDeviceDescription(items.join(" / "))
      setDeviceDetail(deviceDetail);
    })
    .catch(function (error) {
      if (error.response) {
        message.error("Failed to get device " + device_name + "status due to " + error.response.status + " - " + error.response.data['error']);
      }
    })
  }, [API_ENDPOINT, device_name]);

  useEffect(() => {
    updateDeviceDetail()
  }, [updateDeviceDetail])

  const handleMenuClick = (e) => {
    setMenuCurrent(e.key);
    let options = e.key.split(":")
    if (options[0] === "log") {
      // if log source has changed
      if (logSource !== options[1]) {
        if (ws) {
          // stop streming the old log source
          ws.close();
        }
        setLog("");
        setLogSource(options[1]);
      }
    }
  };
  
  const handleDeviceLog = useCallback((e) => {
    setLog( prevLog => {
      // truncate log cache if it exceeds LOG_SIZE_LIMIT
      if (prevLog.length > LOG_SIZE_LIMIT) {
        return (prevLog + e.data).slice(-LOG_SIZE_LIMIT);
      }
      setLog( prevLog => {return prevLog + e.data});
    });
  }, []);

  useEffect(() => {
    if(ws){
      ws.addEventListener("message", handleDeviceLog);
      return () => {
        ws.removeEventListener("message", handleDeviceLog);
        ws.close();
      }
    }
  },[ws, handleDeviceLog]);

  const showInstallerModal = () => {
    setInstallerModalVisible(true);
  };

  const hideInstallerModal = () => {
    setInstallerModalVisible(false);
  };

  function startVM(vm_name) {
    message.info("Booting the device " + vm_name)
    var url = API_ENDPOINT + '/vms/' + vm_name + '/start'
    axios.post(url)
    .then(function (response) {
      // query device status after 10s
      setTimeout(() => {updateDeviceDetail()}, 10000);  
    })
    .catch(function (error) {
      if (error.response) {
        message.error("Failed to start device " + vm_name + " due to " + error.response.status + " - " + error.response.data['error']);
      }
    })
  }

  function stopVM(vm_name) {
    message.info("Stopping the deivce " + vm_name);
    var url = API_ENDPOINT + '/vms/' + vm_name + '/stop'
    axios.post(url)
    .then(function (response) {
      message.success("Device " + vm_name + " stopped successfully")
       // query device status now
       updateDeviceDetail();
    })
    .catch(function (error) {
      if (error.response) {
        message.error("Failed to stop device " + vm_name + " due to " + error.response.status + " - " + error.response.data['error']);
      }
    })
  }

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
            { deviceDescription !== "" && "status" in deviceDetail && deviceDetail["status"] === 1 ?
              <VNCDisplay url={VNC_WS_URL}/>
              :
              <Spin spinning={true} tip="Waiting for device...">
                <Image preview="false" src="/phone-frame.png"/>
              </Spin>
            }
          </Col>
          <Col span={18}>
            <Menu mode="horizontal" onClick={handleMenuClick} selectedKeys={menuCurrent}>
              <Menu.Item key="terminal" icon={<InteractionOutlined />}> Terminal</Menu.Item>
              <SubMenu key="SubMenu" icon={<BarsOutlined />} title="Device Log">
                <Menu.Item key="log:launcher">Launcher</Menu.Item>
                <Menu.Item key="log:kernel">Kernel</Menu.Item>
              </SubMenu>
              <Menu.Item key="files" icon={<FolderOpenOutlined />}> Files </Menu.Item>
              <Menu.Item key="connection" icon={<LaptopOutlined />}> Connection </Menu.Item>
              <Menu.Item key="settings" icon={<SettingOutlined />}> Settings </Menu.Item>
            </Menu>
            <div id="menu-content-terminal" style={{display: menuCurrent==="terminal" ? 'block' : 'none'}}>
              <WebTerminal deviceName={device_name} isHidden={menuCurrent==="terminal" ? false : true}/>
            </div>
            <div id="menu-content-log" style={{display: menuCurrent.startsWith("log") ? 'block' : 'none', height: "100%"}}>
              {/* // TODO ScrollFollow is not working probably known bug pending PR merge
                  // https://github.com/mozilla-frontend-infra/react-lazylog/pull/41/files */}
              <ScrollFollow
                startFollowing={true}
                render={({ follow, onScroll }) => (
                  <LazyLog height={700} text={log} enableSearch stream follow={follow} onScroll={onScroll} />
                )}
              />
            </div>
						{menuCurrent==="files" ? <FileExplorer deviceName={device_name}/> : '' }
						{menuCurrent==="connection" ? 
							<Connection
								cf_instance={cf_instance}
								deviceDetail={deviceDetail}
							/> 
							: '' 
						}
						{menuCurrent==="settings" ? <p>Nothing to setup</p> : '' }
          </Col>
        </Row>
      </QueueAnim>
      <ApkPickerModal 
        visible={installerModalVisible} 
        onCancelCallback={hideInstallerModal} 
        deviceName={device_name}
      />

    </div>
  )
}

export default DeviceDetail;