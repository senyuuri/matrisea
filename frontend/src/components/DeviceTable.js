import { Layout, Breadcrumb, Row, Col, Table, Tag, Space, Badge } from 'antd';
import { Link, BrowserRouter as Router, Route } from 'react-router-dom';

const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
    },
    {
      title: 'Name',
      dataIndex: 'name',
    },
    {
      title: 'AOSP Image',
      dataIndex: "aosp_version",
    },
    {
      title: 'Created At',
      dataIndex: "created",
    },
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
      render: (status) => (
        <>{ 
          <Badge status="success" text="Running" />
        }
        </> 
      )
    },
    {
        title: 'Action',
        render: (text, record) => (
            <Space size="middle">
                <Link to="/device/aaa">View</Link>
                <a>Start/Stop</a>
                <a>Delete</a>
            </Space>
        ),
    },
];

function DeviceTable(props) {
    return (
        <Table style={{ paddingTop: '10px' }} columns={columns} dataSource={props.data} />
    )
}

export default DeviceTable;