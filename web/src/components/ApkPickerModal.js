import { Form, Select, Upload, Modal, Tabs, message} from 'antd';
import { InboxOutlined } from '@ant-design/icons';
import { useEffect, useState, useCallback, useContext } from 'react';
import axios from 'axios';
import { WsContext } from '../Context';

const { Dragger } = Upload;
const { TabPane } = Tabs;


const ApkPickerModal = ({ visible, onCancelCallback, deviceName }) => {
    const API_ENDPOINT = window.location.protocol+ "//"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1"
    const [form] = Form.useForm();
    const ws = useContext(WsContext);

    const [fileList, setFileList] = useState([]);
    const [isFileListLoading, setIsFileListLoading] = useState(false);

    const onOk = () => {
        if (ws && ws.readyState === 1) {
            ws.send(JSON.stringify({
              type: 2,
              data: {
                  name: deviceName,
                  file: form.getFieldValue("filename")
              }
            }));
            message.info('Installing apk in the background')
        } else {
            message.error('Unable to send request due to connection lost. Please try again')
        }
        onCancelCallback();
    };

    const onCancel = () => {
        form.resetFields();
        onCancelCallback();
    };

    const draggerProps = {
        name: 'file',
        multiple: false,
        accept: ".apk",
        action: API_ENDPOINT + "/vms/" + deviceName + "/upload",
        onChange(info) {
            const { status } = info.file;
            if (status !== 'uploading') {
                //console.log(info.file, info.fileList);
            }
            if (status === 'done') {
                message.success(`${info.file.name} file uploaded successfully.`);
                form.setFieldsValue({filename: info.file.name});
            } else if (status === 'error') {
                message.error(`Failed to upload file due to ${ info.file.response.error }`);
            }
        },
        onDrop(e) {
            console.log('Dropped files', e.dataTransfer.files);
        }
    }

    const handleWSMessage = useCallback((e) => {
        var msg = JSON.parse(e.data);
        // type 2: WS_TYPE_INSTALL_APK
        if (msg.type === 2){
            if(msg.has_error) {
                message.error("Failed to install apk due to" + msg.error);
            } else {
                message.success("Successfully installed "+ msg.data.file + " on "  + msg.data.name)
            }
        }
    },[]);

    useEffect(() => {
        if(ws){
          ws.addEventListener("message", handleWSMessage);
          return () => {
            ws.removeEventListener("message", handleWSMessage);
          }
        }
      }, [ws,handleWSMessage]);

    useEffect(() => {
        if (visible) {
            var url = API_ENDPOINT + "/vms/" + deviceName + "/apks"
            var newFileList = []
            axios.get(url).then(function (response) {
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
                setIsFileListLoading(false);
            })
            .catch(function (error) {
                message.error("Failed to retrive the file list");
            })
        } else {
            setIsFileListLoading(true);
        }
    }, [visible, API_ENDPOINT, deviceName])

    return (
        <Modal destroyOnClose={true} title="Choose/Upload APK file" visible={visible} onOk={onOk} onCancel={onCancel}>
            <Tabs defaultActiveKey="1">
                <TabPane tab="Choose file on server" key="1">
                <Form form={form} layout="vertical" name="fileForm">
                    <Form.Item name="filename">
                        <Select
                            showSearch
                            style={{ width: '100%' }}
                            options={fileList}
                            loading={isFileListLoading}
                        />
                    </Form.Item>
                </Form>
                </TabPane>
                <TabPane tab="Upload" key="2">
                    <Dragger {...draggerProps}>
                        <p className="ant-upload-drag-icon"><InboxOutlined /></p>
                        <p className="ant-upload-text">Click or drag .apk to this area to upload</p>
                    </Dragger>
                </TabPane>
            </Tabs>
        </Modal>
    );
};


export default ApkPickerModal;