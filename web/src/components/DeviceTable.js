import { Table, Tag, Space, Badge, Button, Tooltip } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';

import { Link } from 'react-router-dom';

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
        render: (text, record) => (
            <Space size="middle">
                <Link to="/device/aaa">View</Link>
                <Button type="link" onClick={startStopVM()}>Start/Stop</Button>
                <Button type="link" onClick={deleteVM()}>Delete</Button>
            </Space>
        ),
    },
];

function startStopVM() {
  //TODO
}

function deleteVM() {
  //TODO
}

function DeviceTable(props) {
  return (
      <Table style={{ paddingTop: '10px' }} columns={columns} dataSource={props.data} />
  )
}

export default DeviceTable;