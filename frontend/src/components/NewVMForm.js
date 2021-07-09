import { Drawer, Form, Button, Col, Row, Input, Select, Slider, InputNumber, Upload} from 'antd';
import React, { useState, useCallback, useEffect } from 'react';
import { InboxOutlined } from '@ant-design/icons';

const { Option } = Select;
const { Dragger } = Upload;

function NewVMForm(props) {
    const [visible, setVisible] = useState(props.visible);

    useEffect(() => {
        setVisible(props.visible);
      }, [props]);

    const handleClose = useCallback(() => {
        setVisible(false);
        props.onChange();
    }, []);

    return (
        <Drawer
            title="Create a new virtual device"
            width={720}
            onClose={handleClose}
            visible={visible}
            bodyStyle={{ paddingBottom: 80 }}
            footer={
            <div
                style={{
                textAlign: 'right',
                }}
            >
                <Button onClick={handleClose} style={{ marginRight: 8 }}>
                Cancel
                </Button>
                <Button onClick={handleClose} type="primary">
                Submit
                </Button>
            </div>
            }
        >
            <Form layout="vertical" hideRequiredMark>
            <Row gutter={16}>
                <Col span={12}>
                <Form.Item
                    name="name"
                    label="Name"
                    rules={[{ required: true, message: 'Please enter user name' }]}
                >
                    <Input placeholder="Please enter user name" />
                </Form.Item>
                </Col>
                <Col span={12}>
                    <Form.Item
                        name="device"
                        label="Device Type"
                        rules={[{ required: true, message: 'Please choose the type' }]}
                    >
                        <Select placeholder="Please choose the type" disabled={true} defaultValue="cuttlefish-kvm">
                            <Option value="cuttlefish-kvm">cuttlefish-kvm</Option>
                        </Select>
                    </Form.Item>
                </Col>
            </Row>
            <Row gutter={16}>
                <Col span={12}>
                    <Form.Item
                        name="cpu"
                        label="CPU"
                        rules={[{ required: true, message: 'Please choose the type' }]}
                    >
                        <Slider
                            min={1}
                            max={20}
                            value={0}
                        />
                    </Form.Item>
                </Col>
                <Col span={12}>
                <Form.Item
                    name="ram"
                    label="RAM"
                    rules={[{ required: true, message: 'Please enter the size of RAM' }]}
                >
                    <Slider
                        min={1}
                        max={20}
                        value={0}
                    />
                </Form.Item>
                </Col>
            </Row>
            <Row>
                <Form.Item
                    name="images"
                    label="Images"
                    rules={[{ required: true, message: 'Please enter the size of RAM' }]}
                >
                    <Dragger>
                        <p className="ant-upload-drag-icon">
                        <InboxOutlined />
                        </p>
                        <p className="ant-upload-text">Click or drag file to this area to upload</p>
                        <p className="ant-upload-hint">
                        Support for a single or bulk upload. Strictly prohibit from uploading company data or other
                        band files
                        </p>
                    </Dragger>
                </Form.Item>
            </Row>
            </Form>
        </Drawer>
    )
}

export default NewVMForm;