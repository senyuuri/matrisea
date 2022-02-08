import { Table, Tag, Space, Badge, Button, Tooltip, message } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';

import { Link } from 'react-router-dom';


function DeviceTable(props) {
  const axios = require('axios');
  const API_ENDPOINT = window.location.protocol+ "//"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1"

  const columns = [
      {
        title: 'Container ID',
        dataIndex: 'id',
      },
      {
        title: 'Device Name',
        dataIndex: 'name',
      },
      {
        title: 'LAN IP',
        dataIndex: "ip",
      },
      {
        title: 'Created At',
        dataIndex: "created",
      }
      ,{
        title: 'Tags',
        dataIndex: 'tags',
        render: tags => (
          <>
            {tags.map(tag => {
              let color = 'geekblue';
              if (tag === 'Custom Kernel') {
                color = 'volcano';
              }
              return (
                <Tag color={color} key={tag}>
                  {tag.toUpperCase()}
                </Tag>
              );
            })}
          </>
        ),
      },
      {
        title: 'Status',
        dataIndex: "status",
        render: (status) => {
          if (status === 0) { // VMReady
            return <Badge status="default" text="Power Off" />
          } 
          else if (status === 1){ // VMRunning
            return <Badge status="success" text="Running" />
          }
          else if (status === 2){ // VMContainerError
            return <>
              <Badge status="error" text="Error" />
              <Tooltip placement="top" title="Unexpected state of VM container. Contact Admin to resolve">
                <Badge id="vm-status-error-tooltip" count={<QuestionCircleOutlined />} />
              </Tooltip>
            </>
            
          }
        }
      },
      {
        title: 'Action',
        dataIndex: ["name", "cf_instance"],
        render: (text, row) => {
            let view_link = "/device/" + row["name"] +"/" + row["cf_instance"]
            let actionButton;
            let viewButton;
            if (row["status"] === 0) {
              viewButton = <Link to="#" disabled={true}> View </Link>
              actionButton = <Button type="link" onClick={() => startVM(row["name"])}>Start</Button>
            }
            else if (row["status"] === 1) {
              viewButton = <Link to={view_link} > View </Link>
              actionButton = <Button type="link" onClick={() => stopVM(row["name"])}>Stop</Button>
            }
            else {
              viewButton = <Link to="#" disabled={true}> View </Link>
              actionButton = <Button type="link" disabled={true}>Start</Button>
            }
            return <Space size="middle">
              {viewButton}
              {actionButton}
              <Button type="link" onClick={() => deleteVM(row["name"])}>Delete</Button>
            </Space>
        },
      },
  ];

  function startVM(vm_name) {
    message.info("Booting the device " + vm_name)
    var url = API_ENDPOINT + '/vms/' + vm_name + '/start'
    axios.post(url)
    .then(function (response) {
    })
    .catch(function (error) {
      if (error.response) {
        message.error("Failed to start device " + vm_name + " due to " + error.response.status + " - " + error.response.data['error']);
      }
    })
  }

  function stopVM(vm_name) {
    message.info("Stopping the deivce " + vm_name)
    var url = API_ENDPOINT + '/vms/' + vm_name + '/stop'
    axios.post(url)
    .then(function (response) {
      message.success("Device " + vm_name + " stopped successfully")
    })
    .catch(function (error) {
      if (error.response) {
        message.error("Failed to stop device " + vm_name + " due to " + error.response.status + " - " + error.response.data['error']);
      }
    })
  }

  function deleteVM(vm_name) {
    message.info("Requesting to delete " + vm_name)
    var url = API_ENDPOINT + '/vms/' + vm_name
    axios.delete(url)
    .then(function (response) {
      message.success("Device " + vm_name + " has been destroyed")
    })
    .catch(function (error) {
      console.error(error);
      if (error.response) {
        message.error("Failed to delete device " + vm_name + " due to " + error.response.status + " - " + error.response.data['error']);
      }
    })
  }

  return (
    <Table 
      style={{ paddingTop: '10px' }} 
      columns={columns} 
      dataSource={props.data} 
      loading={props.data.length === 0 ? true: false}
    />
  )
}

export default DeviceTable;