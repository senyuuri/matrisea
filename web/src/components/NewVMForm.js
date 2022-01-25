import { Drawer, Form, Button, Col, Row, Input, Select, Upload, Modal, Tabs, Divider, Steps, message} from 'antd';
import React, { useState, useCallback, useEffect, useRef, useReducer } from 'react';
import { PlusOutlined, InboxOutlined, CheckOutlined, LoadingOutlined, CloseCircleOutlined} from '@ant-design/icons';
import { WsContext } from '../Context';

const { TabPane } = Tabs;
const { Dragger } = Upload;
const { Option } = Select;
const { Step } = Steps;

function NewVMForm(props) {
  const axios = require('axios');
  const API_ENDPOINT = window.location.protocol+ "//"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1"
  const ws = React.useContext(WsContext);

  const [form] = Form.useForm();
  const [fileModalVisible, setFileModalVisible] = useState(false);
	const [visible, setVisible] = useState(props.visible);
  const [filePickerType, setFilePickerType] = useState('System');
  const [fileList, setFileList] = useState([]);
  const [systemImageButtonText, setSystemImageButtonText] = useState('Select File');
  const [systemImageIcon, setSystemImageIcon] = useState('PlusOutlined');
  const [cvdImageButtonText, setCvdImageButtonText] = useState('Select File');
  const [cvdImageIcon, setCvdImageIcon] = useState('PlusOutlined');
  const [currentStep, setCurrentStep] = useState(0);
  const [currentCreateVMStep, setCurrentCreateVMStep] = useState(0);
  const [hasErrorInCreateVMStep, setHasErrorInCreateVMStep] = useState(false);
  const [step1Visible, setStep1Visible] = useState(true);
  const [step2Visible, setStep2Visible] = useState(false);
  const [step3Visible, setStep3Visible] = useState(false);
  const [stepStartTime, setStepStartTime] = useState();

  const [stepMessages, setStepMessages] = useReducer((stepMessages, { type, idx, value }) => {
    switch (type) {
      case "update":
        let tmpArr = [...stepMessages];
        tmpArr[idx] = value;
        return tmpArr;
      default:
        return stepMessages;
    }
  }, Array(5).fill(''));

  const VMCreationSteps = [
    "Request Submitted", "Preflight Checks", "Create VM", "Load Images", "Start VM"
  ]

  useEffect(() => {
    if(ws){
      ws.addEventListener("message", handleWSMessage);
      return () => {
        ws.removeEventListener("message", handleWSMessage);
      }
    }
  }, [ws,stepStartTime]);

  const handleWSMessage = (e) => {
    var msg = JSON.parse(e.data);
    // type 1: WS_TYPE_CREATE_VM
    if (msg.type == 1){
      if(msg.has_error) {
        setHasErrorInCreateVMStep(true);
        setStepMessages({type:"update", idx: msg.step, value: msg.error});
      } else {
        // step in msg means the previously completed step, so we +1 to advance 
        setCurrentCreateVMStep(msg.step+1)
        let diff = Math.round(new Date().getTime() / 1000 - stepStartTime) + 1
        let time_cost = Math.round(diff/ 60) + 'm ' + diff % 3 + 's' 
        setStepMessages({type:"update", idx: msg.step, value: time_cost });
        console.log('step_message',stepStartTime,{type:"update", idx: msg.step, value: time_cost });
        setStepStartTime(new Date().getTime() / 1000);
  
        if (msg.step + 1 === 3) {
          setStepMessages({type:"update", idx: 3, value: "Depends on the size of images, this may take 2-3 minutes..." });
        }
      }
    }
  }

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
    axios.get(API_ENDPOINT + "/files/system")
    .then(function (response) {
      var files = response.data.files;
      if(files != null ){
        files.forEach(f => {
          newFileList.push({
            value: f, 
            name: f
          });
        })
      }
      setFileList(newFileList);
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
    axios.get(API_ENDPOINT + "/files/cvd")
    .then(function (response) {
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
    })
    .catch(function (error) {
      console.error(error);
      message.error("Failed to retrive image info");
    })
    setFilePickerType('CVD');
    showFileModal();
  }

  const resetForm = () => {
    form.resetFields();
    setSystemImageButtonText('Select File');
    setSystemImageIcon('PlusOutlined');
    setCvdImageButtonText('Select File');
    setCvdImageIcon('PlusOutlined');
    setCurrentStep(0);
    setCurrentCreateVMStep(0);
    setStep1Visible(true);
    setStep2Visible(false);
    setStep3Visible(true);
  }

  const handleClose = useCallback((values) => {
		setVisible(false);
    resetForm();
		props.onChange();
	}, []);

	const submitForm = useCallback((values) => {
    if (ws && ws.readyState == 1) {
      ws.send(JSON.stringify({
        type: 'create',
        data: values
      }));
    } else {
      // TODO try reconnect ws
      message.error('Disconnected from the server. Refresh and try again!')  
    }
    
		setCurrentStep(1);
    setStep1Visible(false);
    setStep2Visible(true);
    setStep3Visible(false);
    setStepStartTime(new Date().getTime() / 1000);
    console.log("currentstepstarttime", new Date().getTime() / 1000);
	});

  const onDeviceCreationSuccess = () => {
    setVisible(false);
    resetForm();
		props.onChange();
  }
  
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
          <Button form='new-device-form' htmlType="submit" type="primary">Submit</Button>
        </div>
			}
		>
      <Steps current={currentStep} size="small">
        <Step key="Configure" title="Configure" />
        <Step key="Initialize Device" title="Initialize Device" />
        <Step key="Done" title="Done" />
      </Steps>
      <Divider />
      <div id='step-1-div' style={{display: step1Visible ? 'block' : 'none'}}>
        <Form.Provider
          // when the file picker modal is closed submitted, the file name
          // will be synchronised to the main form
          onFormFinish={(name, { values, forms }) => {
            if (name === 'fileForm') {
              const { mainForm } = forms;
              if(filePickerType === "System") {
                mainForm.setFieldsValue({
                  system_image: values.filename
                });
                setSystemImageButtonText(values.filename);
                setSystemImageIcon('CheckOutlined');
              }
              else if (filePickerType === "CVD") {
                mainForm.setFieldsValue({
                  cvd_image: values.filename
                })
                setCvdImageButtonText(values.filename);
                setCvdImageIcon('CheckOutlined');
              }
              setFileModalVisible(false);
            }
          }}
        >
          <Form 
            layout="vertical"
            id="new-device-form"
            name="mainForm"
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
                    name="system_image"
                    rules={[{ required: true, message: 'Please upload/choose a system image' }]}
                  >
                    <Input hidden/>
                  </Form.Item>
                  <Form.Item noStyle>
                  <Button
                    type="dashed"
                    onClick={chooseSystemFile}
                    style={{ width: '100%' }}
                    icon={ systemImageIcon === 'PlusOutlined'?<PlusOutlined />:<CheckOutlined/>}
                  >
                    {systemImageButtonText}
                  </Button>
                  </Form.Item>
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item label="CVD Image">
                  <Form.Item
                    noStyle
                    name="cvd_image"
                    rules={[{ required: true, message: 'Please upload/choose a CVD image' }]}
                  >
                    <Input hidden/>
                  </Form.Item>
                  <Form.Item noStyle>
                    <Button
                      type="dashed"
                      onClick={chooseCVDFile}
                      style={{ width: '100%' }}
                      icon={ cvdImageIcon === 'PlusOutlined'?<PlusOutlined />:<CheckOutlined/>}
                    >
                      {cvdImageButtonText}  
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
      </div>
      <div id='step-2-div' style={{display: step2Visible ? 'block' : 'none'}}>
       
        <Steps current={currentCreateVMStep} direction="vertical">
          {VMCreationSteps.map((step,idx) => (
            <Step 
              title={step} 
              icon={currentCreateVMStep === idx ? (!hasErrorInCreateVMStep? <LoadingOutlined />:'') : ''} 
              description={stepMessages[idx]}
              status={currentCreateVMStep === idx ? (hasErrorInCreateVMStep? 'error':''):'' }
            />
          ))}
        </Steps>
      </div>
      
		</Drawer>
	)
}

const FileModalForm = ({ visible, onCancel, target, fileList }) => {
  const API_ENDPOINT = window.location.protocol+ "//"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1"
  const [form] = Form.useForm();
  
  const onOk = () => {
    form.submit();
    onCancel();
  };

  const draggerProps = {
    name: 'file',
    multiple: false,
    accept: ".zip,.tar",
    action: API_ENDPOINT + "/files/upload",
    onChange(info) {
      const { status } = info.file;
      if (status !== 'uploading') {
        //console.log(info.file, info.fileList);
      }
      if (status === 'done') {
        message.success(`${info.file.name} file uploaded successfully.`);
        form.setFieldsValue({filename: info.file.name});
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
          <Form form={form} layout="vertical" name="fileForm">
            <Form.Item name="filename">
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