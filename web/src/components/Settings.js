import { useState } from 'react';
import { Menu, Row, Col, Typography, Table, Form, Input, Button, message} from 'antd';
import {InfoCircleOutlined, SettingOutlined} from '@ant-design/icons';
import axios from 'axios';

const { Text } = Typography;

function Settings({deviceName, deviceDetail}){
  const API_ENDPOINT = window.location.protocol+ "//"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1"
  const [currentPage, setCurrentPage] = useState("settings");
  console.log(deviceDetail);

  const columns = [
    {
      title: 'Key',
      dataIndex: 'key',
      width: 200,
    },{
      title: 'Value',
      dataIndex: 'value',
    }
  ]

  const processDeviceDetail = () => {
    if (!deviceDetail) {
      return []
    }
    return [
      {key: "Device Name", value: deviceDetail['name']},
      {key: "OS Version", value: deviceDetail['os_version']},
      {key: "vCPU", value: deviceDetail['cpu']},
      {key: "RAM", value: deviceDetail['ram'] + " GB"},
      {key: "Emulator", value: "cuttlefish"},
      {key: "Workspace Container", value: 'matrisea-cvd-' + deviceDetail['name']},
      {key: "Container ID", value: deviceDetail["id"]},
      {key: "Container IP", value: deviceDetail['ip']},
      {key: "Created At", value: new Date(parseInt(deviceDetail['created'])*1000).toLocaleString()},
      {key: "Cuttlefish Instance ID", value: deviceDetail["cf_instance"]},
      {key: "Display Mode", value: "vnc"},
    ]
  };

  const tableData = processDeviceDetail();
  
  const onMenuSelect = (e) => {
    setCurrentPage(e.key);
  };

  const submitForm = (values) => {
    console.log(values);
    var url = API_ENDPOINT + '/vms/' + deviceName + '/config'
    axios.post(url, {key: 'cmdline', value: values.cmdline})
    .then(function (response) {
      message.success("Update success. The device must reboot for the change to take effect.")
    })
    .catch(function (error) {
      if (error.response) {
        message.error("Failed to update device config. Reason: " + error.response.data['message']);
      }
    })
  } 

	return (
		<div id="menu-content-settings" className="detail-tab-content">
			<Row>
        <Col span={4}> 
          <Menu
            defaultSelectedKeys={['settings']}
            mode="inline"
            onSelect={onMenuSelect}
          >
            <Menu.Item key="settings" icon={<SettingOutlined />}>Settings</Menu.Item>
            <Menu.Item key="info" icon={<InfoCircleOutlined />}>System Information</Menu.Item>
          </Menu>
        </Col>
        <Col span={20}>
          { currentPage === "settings" ?
            <div className="settings-page">
              <h4><Text code>launch_cvd</Text> Options</h4>
              <Form 
                name="main-form" 
                onFinish={submitForm}
                initialValues={{
                  cmdline: deviceDetail ? deviceDetail['cmdline'] : "",
                }}
              >
                <Form.Item name="cmdline" >
                  <Input.TextArea spellCheck="false" style={{fontFamily: "Courier New"}}/>
                </Form.Item>
              </Form>
              <Row>
                <Col span={22}></Col>
                <Col span={2}>
                  <Button form='main-form' htmlType="submit" type="primary">Update</Button>
                </Col>
              </Row> 
            </div>
            : ""
          }

          { currentPage === "info" ?
            <div className="settings-page">
              <h4>System Information</h4>
              <Table 
                className="file-explorer"
                columns={columns} 
                dataSource={tableData} 
                pagination={false} 
                showHeader={false}
              />
            </div>
            :""
          }
        </Col>
			</Row>
		</div>
	)
}

export default Settings;