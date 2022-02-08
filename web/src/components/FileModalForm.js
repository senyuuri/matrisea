import { Form, Select, Upload, Modal, Tabs, message} from 'antd';
import { InboxOutlined } from '@ant-design/icons';

const { Dragger } = Upload;
const { TabPane } = Tabs;


const FileModalForm = ({ visible, onCancelCallback, target, fileList }) => {
    const API_ENDPOINT = window.location.protocol+ "//"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1"
    const [form] = Form.useForm();

    const onOk = () => {
        // reset this form in the main form's onFormFinish()
        form.submit();
        onCancelCallback();
    };

    const onCancel = () => {
        form.resetFields();
        onCancelCallback();
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


export default FileModalForm;