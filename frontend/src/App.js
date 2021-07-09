import React, { useState, useCallback, useEffect } from 'react';
import NewVMForm from './components/NewVMForm';
import './App.css';
import { Button } from 'antd';
import { Layout, Menu, Breadcrumb, Row, Col, Table, Tag, Space } from 'antd';
const { Header, Footer, Sider, Content } = Layout;

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
        <Tag color='green' key={status}>
            {status}
        </Tag>
      }
      </> 
    )
  },
  {
    title: 'Action',
    render: (text, record) => (
      <Space size="middle">
        <a>View</a>
        <a>Start/Stop</a>
        <a>Delete</a>
      </Space>
    ),
  },
];

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

function App() {
  const [formVisible, setFormVisible] = useState(false);
  function handleFormClose() {
    setFormVisible(false);
  }

  useEffect(() => {
    document.title = "Matrisea"
  }, []);

  return (<Layout className="layout" style={{ minHeight: "100vh" }}>
    <Header>
      <div className="logo">
        <img src="/logo512.png" style={{maxWidth: "100%", maxHeight: "100%" }}></img>
      </div>
      {/* <h1 className="logo-text"> Matrisea</h1> */}
    </Header>
    <Content style={{ padding: '0 50px' }}>
      <div className="site-layout-content">
        <Row justify="space-between">

            <Breadcrumb>
              <Breadcrumb.Item>Home</Breadcrumb.Item>
              <Breadcrumb.Item>Devices</Breadcrumb.Item>
            </Breadcrumb>
        
            <Button onClick={() => {console.log(1111); setFormVisible(true);}}>New Virtual Device</Button>
        </Row>
        <Table style={{ paddingTop: '10px' }} columns={columns} dataSource={data} />
      </div>
      <NewVMForm visible={formVisible} onChange={handleFormClose}/>
    </Content>
  </Layout>)
}

export default App;
