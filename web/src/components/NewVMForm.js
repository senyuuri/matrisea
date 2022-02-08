import { Drawer, Form, Button, Col, Row, Input, Select, Divider, Steps, message} from 'antd';
import React, { useState, useCallback, useEffect, useReducer} from 'react';
import { PlusOutlined, CheckOutlined, LoadingOutlined} from '@ant-design/icons';
import { WsContext } from '../Context';
import FileModalForm from './FileModalForm';

const { Option } = Select;
const { Step } = Steps;

function NewVMForm(props) {
  const axios = require('axios');
  const API_ENDPOINT = window.location.protocol+ "//"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1"
  const ws = React.useContext(WsContext);
  
  const [form] = Form.useForm();

  // State of Drawer
	const [visible, setVisible] = useState(props.visible);
  const [isMaskClosable, setIsMaskClosable] = useState(true);

  // State of Global Steps
  const [currentStep, setCurrentStep] = useState(1);

  // State of Step 1 - New VM Form
  const [systemImageButtonText, setSystemImageButtonText] = useState('Select File');
  const [systemImageIcon, setSystemImageIcon] = useState('PlusOutlined');
  const [cvdImageButtonText, setCvdImageButtonText] = useState('Select File');
  const [cvdImageIcon, setCvdImageIcon] = useState('PlusOutlined');

  // State of Step 2 - VM Creation Progress
  const [fileModalVisible, setFileModalVisible] = useState(false);
  const [filePickerType, setFilePickerType] = useState('System');
  const [fileList, setFileList] = useState([]);
  const [currentCreateVMStep, setCurrentCreateVMStep] = useState(0);
  const [hasErrorInCreateVMStep, setHasErrorInCreateVMStep] = useState(false);
  const [stepStartTime, setStepStartTime] = useState();
  const [hasVMCreationSucceed, setHasVMCreationSucceed] = useState(false);
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

  // reset the form and states when the modal is closed
  const resetForm = useCallback(() => {
    form.resetFields();
    setSystemImageButtonText('Select File');
    setSystemImageIcon('PlusOutlined');
    setCvdImageButtonText('Select File');
    setCvdImageIcon('PlusOutlined');
    setCurrentStep(1);
    setCurrentCreateVMStep(0);
    setIsMaskClosable(true);
    setHasErrorInCreateVMStep(false);
    setHasVMCreationSucceed(false);
  },[form]);

  const handleWSMessage = useCallback((e) => {
    var msg = JSON.parse(e.data);
    // type 1: WS_TYPE_CREATE_VM
    if (msg.type === 1){
      if(msg.has_error) {
        setHasErrorInCreateVMStep(true);
        setStepMessages({type:"update", idx: msg.data.step, value: msg.error});
      } else {
        // step in msg means the server acknowledge that the previous step has been completed, so we +1 to advance 
        setCurrentCreateVMStep(msg.data.step+1)
        let diff = Math.round(new Date().getTime() / 1000 - stepStartTime) + 1
        let time_cost = Math.round(diff/ 60) + 'm ' + diff % 3 + 's' 
        setStepMessages({type:"update", idx: msg.data.step, value: time_cost });
        console.log('step_message',stepStartTime,{type:"update", idx: msg.data.step, value: time_cost });
        setStepStartTime(new Date().getTime() / 1000);
  
        if (msg.data.step + 1 === 3) {
          setStepMessages({type:"update", idx: 3, value: "Depends on the size of images, this may take 2-3 minutes..." });
        }
        if (msg.data.step + 1 === 4) {
          setHasVMCreationSucceed(true);
        }
      }
    }
  },[stepStartTime]);

  useEffect(() => {
    if(ws){
      ws.addEventListener("message", handleWSMessage);
      return () => {
        ws.removeEventListener("message", handleWSMessage);
      }
    }
  }, [ws,stepStartTime,handleWSMessage]);

	useEffect(() => {
		setVisible(props.visible);
	}, [props]);

  const showFileModal = () => {
    setFileModalVisible(true);
  };

  const hideFileModal = () => {
    setFileModalVisible(false);
  };

  const handleClose = useCallback((values) => {
		setVisible(false);
    resetForm();
		props.onChange();
	}, [props, resetForm]);

  // Progress form step 1 to 2
	const submitForm = useCallback((values) => {
    if (ws && ws.readyState === 1) {
      ws.send(JSON.stringify({
        type: 1,
        data: values
      }));
    } else {
      message.error('Failed to submit request due to connection lost. Please try again')
      return
    }

		setCurrentStep(2);
    setIsMaskClosable(false);
    setStepStartTime(new Date().getTime() / 1000);
	},[ws]);

  // Progress form step 2 to 3
  const viewResults = () => {
    setCurrentStep(3);
    setIsMaskClosable(true);
  }

  const chooseSystemFile = () => {
    getFileList("system")
    setFilePickerType('System');
    showFileModal();
  }

  const chooseCVDFile = () => {
    getFileList("cvd")
    setFilePickerType('CVD');
    showFileModal();
  }

  const getFileList = (filetype) => {
    var url = API_ENDPOINT + "/files/" + filetype
    var newFileList = []
    axios.get(url)
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
  }

	return (
		<Drawer
			title="Create a new virtual device"
			width={720}
			onClose={handleClose}
			visible={visible}
			bodyStyle={{ paddingBottom: 80 }}
      maskClosable={isMaskClosable}
			footer={
        <div style={{textAlign: 'right',}}>
          <div id='step-1-buttons' style={{display: currentStep===1 ? 'block' : 'none'}}>
            <Button onClick={handleClose} style={{ marginRight: 8 }}>Cancel</Button>
            <Button form='new-device-form' htmlType="submit" type="primary">Submit</Button>
          </div>
          <div id='step-2-buttons' style={{display: currentStep===2 ? 'block' : 'none'}}>
            <Button onClick={handleClose} style={{ marginRight: 8 }}>Cancel</Button>
            <Button onClick={viewResults} disabled={!hasVMCreationSucceed}>Next</Button>
          </div>
          <div id='step-3-buttons' style={{display: currentStep===3 ? 'block' : 'none'}}>
            <Button onClick={handleClose} style={{ marginRight: 8 }}>Done</Button>
          </div>
        </div>
			}
		>
      <Steps current={currentStep-1} size="small">
        <Step title="Configure" />
        <Step key="Initialize Device" title="Initialize Device" />
        <Step key="Done" title="Done" />
      </Steps>
      <Divider />
      <div id='step-1-div' style={{display: currentStep===1 ? 'block' : 'none'}}>
        <Form.Provider
          // when the file picker modal is closed submitted, the file name
          // will be synchronised to the main form
          onFormFinish={(name, { values, forms }) => {
            if (name === 'fileForm') {
              const { mainForm, fileForm } = forms;
              if(filePickerType === "System") {
                mainForm.setFieldsValue({
                  system_image: values.filename
                });
                if (values.filename.length > 35) {
                  values.filename = values.filename.slice(0,15) + "..." + values.filename.slice(-12)
                } 
                setSystemImageButtonText(values.filename);
                setSystemImageIcon('CheckOutlined');
              }
              else if (filePickerType === "CVD") {
                mainForm.setFieldsValue({
                  cvd_image: values.filename
                })
                if (values.filename.length > 35) {
                  values.filename = values.filename.slice(0,15) + "..." + values.filename.slice(-12)
                } 
                setCvdImageButtonText(values.filename);
                setCvdImageIcon('CheckOutlined');
              }
              fileForm.resetFields();
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
              name: 'cvd-'+ Math.random().toString(36).substring(2, 8),
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
            onCancelCallback={hideFileModal} 
            target={filePickerType}
            fileList={fileList}
          />
        </Form.Provider>
      </div>

      <div id='step-2-div' style={{display: currentStep===2 ? 'block' : 'none'}}>
        <Steps current={currentCreateVMStep} direction="vertical">
          {VMCreationSteps.map((step,idx) => (
            <Step 
              key={idx}
              title={step} 
              icon={currentCreateVMStep === idx ? (!hasErrorInCreateVMStep? <LoadingOutlined />:'') : ''} 
              description={stepMessages[idx]}
              status={currentCreateVMStep === idx ? (hasErrorInCreateVMStep? 'error':''):'' }
            />
          ))}
        </Steps>
      </div>

      <div id='step-3-div' style={{display: currentStep===3 ? 'block' : 'none'}}>
        <p>Success!</p>
      </div>
      
		</Drawer>
	)
}

export default NewVMForm;