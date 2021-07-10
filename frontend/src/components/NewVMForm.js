import { Upload, Drawer, Form, Button, Col, Row, Input, Select, Slider, InputNumber, Space} from 'antd';
import React, { useState, useCallback, useEffect } from 'react';
import { UploadOutlined, InboxOutlined } from '@ant-design/icons';

const { Option } = Select;
const { Dragger } = Upload;

const fileList = [
    {
      uid: '-2',
      name: 'aosp-xxxxxxx.img',
      status: 'success',
    },
  ];

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
                    label="Device Name"
                    rules={[{ required: true, message: 'Please enter a device name' }]}
                >
                    <Input placeholder="Virtual device name" />
                </Form.Item>
                </Col>
                <Col span={12}>
                    <Form.Item
                        name="device"
                        label="Device Type"
                        rules={[{ required: true, message: 'Please choose the device type' }]}
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
                        rules={[{ required: true, message: 'Please choose the CPU' }]}
                    >
                        <Select placeholder="Please choose the CPU"  defaultValue={2}>
                            {new Array(4).fill(null).map((_, index) => {
                                const key = index + 1;
                                return <Option value={key}> {key} vCPU</Option>
                                
                            })}
                            
                        </Select>
                    </Form.Item>
                </Col>
                <Col span={12}>
                <Form.Item
                    name="ram"
                    label="RAM"
                    rules={[{ required: true, message: 'Please enter the size of RAM' }]}
                >
                    <Select placeholder="Please choose the size of RAM"  defaultValue={4}>
                        {new Array(8).fill(null).map((_, index) => {
                            const key = index + 1;
                            return <Option value={key}> {key} GB</Option>
                            
                        })}
                        
                    </Select>
                </Form.Item>
                </Col>
            </Row>
            <Row gutter={16}>
                <Col span={12}>
                    <Form.Item
                        name="aosp-image"
                        label="System Image"
                        rules={[{ required: true, message: 'Please upload/choose a system image' }]}
                    >
                        <Upload
                            action="https://www.mocky.io/v2/5cc8019d300000980a055e76"
                            listType="picture"
                            defaultFileList={[...fileList]}
                        >
                            <Space>
                                <Button icon={<UploadOutlined />}>Click to upload</Button>
                                <Button disabled={true} icon={<UploadOutlined />}>Previous uploads</Button>
                            </Space>
                        
                        </Upload>
                        
                    </Form.Item>
                </Col>
                <Col span={12}>
                    <Form.Item
                        name="cvd-image"
                        label="CVD Image"
                        rules={[{ required: true, message: 'Please upload/choose a CVD image' }]}
                    >
                        <Upload
                            action="https://www.mocky.io/v2/5cc8019d300000980a055e76"
                            listType="picture"
                            defaultFileList={[...fileList]}
                        >
                            <Space>
                                <Button icon={<UploadOutlined />}>Click to upload</Button>
                                <Button disabled={true} icon={<UploadOutlined />}>Previous uploads</Button>
                            </Space>
                        
                        </Upload>
                        
                    </Form.Item>
                </Col>
            </Row>
            <Row gutter={16}>
                <Col span={12}>
                    <Form.Item
                        name="kernel-image"
                        label="Kernel Image (Optional)"
                        rules={[{ required: false }]}
                    >
                        <Upload
                            action="https://www.mocky.io/v2/5cc8019d300000980a055e76"
                            listType="picture"
                            defaultFileList={[...fileList]}
                        >
                            <Space>
                                <Button icon={<UploadOutlined />}>Click to upload</Button>
                                <Button disabled={true} icon={<UploadOutlined />}>Previous uploads</Button>
                            </Space>
                        
                        </Upload>
                        
                    </Form.Item>
                </Col>
            </Row>
            </Form>
        </Drawer>
    )
}

export default NewVMForm;