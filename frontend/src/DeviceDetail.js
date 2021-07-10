import { Breadcrumb, Row, Button, } from 'antd';

function DeviceDetail(){
  return (
    <div>
      <div className="site-layout-content">
        <Row justify="space-between">
          <Breadcrumb>
            <Breadcrumb.Item>Home</Breadcrumb.Item>
            <Breadcrumb.Item>Device</Breadcrumb.Item>
            <Breadcrumb.Item>matrisea-aaa-bbb</Breadcrumb.Item>
          </Breadcrumb>
          <Button onClick={() => {}}>Start</Button>
        </Row>
      </div>
    </div>
  )
}

export default DeviceDetail;