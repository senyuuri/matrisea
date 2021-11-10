import { Drawer, Form, Button, Col, Row, Input, Select, Upload, Modal, Tabs, message} from 'antd';
import React, { useState, useCallback, useEffect } from 'react';
import { PlusOutlined, InboxOutlined} from '@ant-design/icons';
const { TabPane } = Tabs;
const { Dragger } = Upload;
const { Option } = Select;

function NewVMForm(props) {
  const axios = require('axios');
  const API_ENDPOINT = process.env.REACT_APP_API_ENDPOINT

  const [form] = Form.useForm();
  const [fileModalVisible, setFileModalVisible] = useState(false);
	const [visible, setVisible] = useState(props.visible);
  const [filePickerType, setFilePickerType] = useState('System')
  const [fileList, setFileList] = useState([])
  
	useEffect(() => {
		setVisible(props.visible);
	}, [props]);
  
  const showFileModal = () => {
    setFileModalVisible(true);
  };

  const hideFileModal = () => {
    setFileModalVisible(false);
  };

  const chooseSystemFile = () => {
    var newFileList = []
    axios.get(`${API_ENDPOINT}/files/system`)
    .then(function (response) {
      console.log(response);
      var files = response.data.files;
      if(files != null ){
        files.forEach(f => {
          newFileList.push({
            value: f, 
            name: f
          });
        })
      }
      setFileList(newFileList)
      console.log(newFileList)
    })
    .catch(function (error) {
      console.error(error);
      message.error("Failed to retrive image info");
    })
    setFilePickerType('System');
    showFileModal();
  }

  const chooseCVDFile = () => {
    var newFileList = []
    axios.get(`${API_ENDPOINT}/files/cvd`)
    .then(function (response) {
      console.log(response);
      var files = response.data.files;
      if(files != null ){
        files.forEach(f => {
          newFileList.push({
            value: f, 
            name: f
          });
        })
      }
      setFileList(newFileList)
      console.log(newFileList)
    })
    .catch(function (error) {
      console.error(error);
      message.error("Failed to retrive image info");
    })
    setFilePickerType('CVD');
    showFileModal();
  }

  const handleClose = useCallback((values) => {
		setVisible(false);
		props.onChange();
	}, []);

	const submitForm = useCallback((values) => {
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
      <Form.Provider
        onFormFinish={(name, { values, forms }) => {
          if (name === 'userForm') {
            const { basicForm } = forms;
            // const users = basicForm.getFieldValue('users') || [];
            // basicForm.setFieldsValue({
            //   users: [...users, values],
            // });
            setVisible(false);
          }
        }}
      >
        <Form 
          layout="vertical"
          id="new-device-form"
          name="basicForm"
          form={form}
          hideRequiredMark
          onFinish={submitForm}
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
                label="System Image"
              >
                <Form.Item 
                  noStyle
                  name="system-image"
                  rules={[{ required: false, message: 'Please upload/choose a system image' }]}
                >
                  <Input hidden/>
                </Form.Item>
                <Form.Item noStyle>
                <Button
                  type="dashed"
                  onClick={chooseSystemFile}
                  style={{ width: '100%' }}
                  icon={<PlusOutlined />}
                >
                  Select File
                </Button>
                </Form.Item>
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item label="CVD Image">
                <Form.Item
                  noStyle
                  name="CVD Image"
                  rules={[{ required: false, message: 'Please upload/choose a CVD image' }]}
                >
                  <Input hidden/>
                </Form.Item>
                <Form.Item noStyle>
                  <Button
                    type="dashed"
                    onClick={chooseCVDFile}
                    style={{ width: '100%' }}
                    icon={<PlusOutlined />}
                    >
                      Select File
                  </Button>
                </Form.Item>
              </Form.Item>
            </Col>
          </Row>
        </Form>
        <FileModalForm 
          visible={fileModalVisible} 
          onCancel={hideFileModal} 
          target={filePickerType}
          fileList={fileList}
        />
      </Form.Provider>
		</Drawer>
	)
}


const FileModalForm = ({ visible, onCancel, target, fileList }) => {
  const API_ENDPOINT = process.env.REACT_APP_API_ENDPOINT
  const [form] = Form.useForm();

  const onOk = () => {
    form.submit();
  };

  const draggerProps = {
    name: 'file',
    multiple: false,
    accept: ".zip,.tar",
    action: `${API_ENDPOINT}/files/upload`,
    onChange(info) {
      const { status } = info.file;
      if (status !== 'uploading') {
        console.log(info.file, info.fileList);
      }
      if (status === 'done') {
        message.success(`${info.file.name} file uploaded successfully.`);
        console.log('success', info)
      } else if (status === 'error') {
        message.error(`${info.file.name} file upload failed.`);
      }
    },
    onDrop(e) {
      console.log('Dropped files', e.dataTransfer.files);
    }
  }

  return (
    <Modal title={"Select " +target +" File"} visible={visible} onOk={onOk} onCancel={onCancel}>

      <Tabs defaultActiveKey="1">
          <TabPane tab="Choose Image" key="1">
          <Form form={form} layout="vertical" name="userForm">
            <Form.Item
              name="name"
              rules={[{ required: true, },]}
            >
              <Select
                showSearch
                style={{ width: '100%' }}
                options={fileList}
              />
            </Form.Item>
          </Form>
          </TabPane>
          <TabPane tab="Upload New Image" key="2">
          <Dragger {...draggerProps}>
            <p className="ant-upload-drag-icon">
              <InboxOutlined />
            </p>
            <p className="ant-upload-text">Click or drag .zip/.tar to this area to upload</p>
          </Dragger>
          </TabPane>
          <TabPane tab="Pick from Android CI" key="3" disabled={true}>
            Content of Tab Pane 3
          </TabPane>
      </Tabs>
      
    </Modal>
  );
};

export default NewVMForm;