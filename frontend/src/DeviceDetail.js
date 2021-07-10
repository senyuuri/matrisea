import { Breadcrumb, Row, Button, PageHeader, Descriptions } from 'antd';
import QueueAnim from 'rc-queue-anim';

function DeviceDetail(){
  return (
    <div key="device-detail">
      <div className="site-layout-content">
        <QueueAnim key="content" type={['right', 'left']}>
          <Row justify="space-between" key="1">
            <Breadcrumb>
              <Breadcrumb.Item>Home</Breadcrumb.Item>
              <Breadcrumb.Item>Device</Breadcrumb.Item>
              <Breadcrumb.Item>matrisea-aaa-bbb</Breadcrumb.Item>
            </Breadcrumb>
          </Row>
          <PageHeader
            key="2"
            ghost={false}
            onBack={() => window.history.back()}
            title="Title"
            subTitle="This is a subtitle"
            extra={[
              <Button key="3">Operation</Button>,
              <Button key="2">Operation</Button>,
              <Button key="1" type="primary">
                Primary
              </Button>,
            ]}
          >
            <Descriptions size="small" column={3}>
              <Descriptions.Item label="Created">Lili Qu</Descriptions.Item>
              <Descriptions.Item label="Association">
                <a>421421</a>
              </Descriptions.Item>
              <Descriptions.Item label="Creation Time">2017-01-10</Descriptions.Item>
              <Descriptions.Item label="Effective Time">2017-10-10</Descriptions.Item>
              <Descriptions.Item label="Remarks">
                Gonghu Road, Xihu District, Hangzhou, Zhejiang, China
              </Descriptions.Item>
            </Descriptions>
          </PageHeader>
        </QueueAnim>
      </div>
    </div>
  )
}

export default DeviceDetail;