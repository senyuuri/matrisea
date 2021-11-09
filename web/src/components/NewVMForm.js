import { Drawer, Form, Button, Col, Row, Input, Select, Space} from 'antd';
import React, { useState, useCallback, useEffect } from 'react';
import { UploadOutlined} from '@ant-design/icons';

const { Option } = Select;

const fileList = [
	{
	  uid: '-2',
	  name: 'aosp-xxxxxxx.img',
	  status: 'success',
	},
  ];

function NewVMForm(props) {
  const [form] = Form.useForm();
	const [visible, setVisible] = useState(props.visible);
  const axios = require('axios');
  const API_ENDPOINT = process.env.REACT_APP_API_ENDPOINT

	useEffect(() => {
		setVisible(props.visible);
	}, [props]);

	const handleClose = useCallback((values) => {
    console.log(values);
    axios({
      method: 'post',
      url: `${API_ENDPOINT}/vms`,
      data: values
    })
    .then(function (response) {
      console.log(response);
    })
    .catch(function (error) {
      console.log(error);
    });

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
        <div style={{textAlign: 'right',}}>
          <Button onClick={handleClose} style={{ marginRight: 8 }}>Cancel</Button>
          <Button form='new-device-form' htmlType="submit">Submit</Button>
        </div>
			}
		>
			<Form 
				layout="vertical"
        id="new-device-form"
				form={form}
				hideRequiredMark
        onFinish={handleClose}
        initialValues={{
          type: "cuttlefish-kvm",
          cpu: 2,
          ram: 4,
        }}
			>
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
							name="type"
							label="Device Type"
							rules={[{ required: true, message: 'Please choose the device type' }]}
						>
							<Select placeholder="Please choose the type" disabled={true}>
								<Option key="cuttlefish-kvm" value="cuttlefish-kvm">cuttlefish-kvm</Option>
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
							<Select placeholder="Please choose the CPU">
								{new Array(4).fill(null).map((_, index) => {
									const key = index + 1;
									return <Option key={key} value={key}> {key} vCPU</Option>
									
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
						<Select placeholder="Please choose the size of RAM">
							{new Array(8).fill(null).map((_, index) => {
								const key = index + 1;
								return <Option key={key} value={key}> {key} GB</Option>
								
							})}
							
						</Select>
					</Form.Item>
					</Col>
				</Row>
				<Row gutter={16}>
					<Col span={12}>
						<Form.Item
							name="system-image"
							label="System Image"
							rules={[{ required: false, message: 'Please upload/choose a system image' }]}
						>
							{/* <Upload
								action="https://www.mocky.io/v2/5cc8019d300000980a055e76"
								listType="picture"
								defaultFileList={[...fileList]}
							>
								<Space>
									<Button icon={<UploadOutlined />}>Click to upload</Button>
									<Button disabled={true} icon={<UploadOutlined />}>Previous uploads</Button>
								</Space>
							
							</Upload> */}
							
						</Form.Item>
					</Col>
					<Col span={12}>
						<Form.Item
							name="cvd-image"
							label="CVD Image"
							rules={[{ required: false, message: 'Please upload/choose a CVD image' }]}
						>
							{/* <Upload
								action="https://www.mocky.io/v2/5cc8019d300000980a055e76"
								listType="picture"
								defaultFileList={[...fileList]}
							>
								<Space>
									<Button icon={<UploadOutlined />}>Click to upload</Button>
									<Button disabled={true} icon={<UploadOutlined />}>Previous uploads</Button>
								</Space>
							
							</Upload>
							 */}
						</Form.Item>
					</Col>
				</Row>
			</Form>
		</Drawer>
	)
}

export default NewVMForm;