import React, { useEffect, useState, useMemo, useCallback} from 'react';
import { useParams, useHistory } from "react-router-dom";
import { Menu, Breadcrumb, Row, Col, Button, PageHeader, Spin, Image, message } from 'antd';
import { PoweroffOutlined, SettingOutlined, InteractionOutlined, BarsOutlined, CloudUploadOutlined } from '@ant-design/icons';
import QueueAnim from 'rc-queue-anim';
import { LazyLog, ScrollFollow } from 'react-lazylog';
import axios from 'axios';

import WebTerminal from './components/Terminal';
import VNCDisplay from './components/VNCDisplay';
import ApkPickerModal from './components/ApkPickerModal';
import FileExplorer from './components/FileExplorer';

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
      subTitle={deviceDescription}
      extra={<>
        <Button icon={<CloudUploadOutlined />} key="install-btn" onClick={showInstallerModal}>Install APK</Button>
        <Button icon={<PoweroffOutlined />} key="power-btn">Power</Button>
      </>}
      {...props}
    />
  ));

  useEffect(() => {
    var url = API_ENDPOINT + '/vms/' + device_name
    axios.get(url)
    .then(function (response) {
      let deviceDetail = response.data;
      const items = [
        deviceDetail['os_version'],
        deviceDetail['cpu'] + " vCPU",
        deviceDetail['ram'] + " GB RAM",
        deviceDetail['ip'],
        deviceDetail['status'],
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
  }, [API_ENDPOINT, device_name])

  const handleMenuClick = (e) => {
    setMenuCurrent(e.key);
    let options = e.key.split(":")
    if (options[0] === "log") {
      if (logSource !== options[1]) {
        if (ws) {
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
              <Menu.Item key="terminal" icon={<InteractionOutlined />}>
                Terminal
              </Menu.Item>
              <SubMenu key="SubMenu" icon={<BarsOutlined />} title="Device Log">
                <Menu.Item key="log:launcher">Launcher</Menu.Item>
                <Menu.Item key="log:kernel">Kernel</Menu.Item>
                <Menu.Item key="log:logcat" disabled={true}>ADB Logcat</Menu.Item>
              </SubMenu>
              <Menu.Item key="files" icon={<SettingOutlined />}>
                Files
              </Menu.Item>
              <Menu.Item key="settings" icon={<SettingOutlined />}>
                Settings
              </Menu.Item>
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
            <div id="menu-content-settings" style={{display: menuCurrent==="settings" ? 'block' : 'none'}}>
              <p>Nothing to setup</p>
            </div>
            <div id="menu-content-settings" style={{display: menuCurrent==="files" ? 'block' : 'none'}}>
              <FileExplorer deviceName={device_name}/>
            </div>
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