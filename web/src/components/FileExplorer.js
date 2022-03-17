import React, { useEffect, useState, useCallback} from 'react';
import { Table, Button, Input} from 'antd';
import {FolderOpenOutlined} from '@ant-design/icons';
import axios from 'axios';
import path from 'path';

function FileExplorer({deviceName}) {
  const API_ENDPOINT = window.location.protocol+ "//"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1"
  const [dir, setDir] = useState("/home/vsoc-01");
  const [tableData, setTableData] = useState([]);

  function gotoDir(next) {
      next = path.normalize(next)
      setDir(next);
  }

  const columns = [
      {
        title: 'Permission',
        dataIndex: 'permission',
        width: 100,
      },{
        title: 'User',
        dataIndex: 'user',
        width: 100,
      },{
        title: 'Group',
        dataIndex: "group",
        width: 100,
      },{
        title: 'Size',
        dataIndex: "size",
        width: 100,
      },{
        title: 'Last Modified',
        dataIndex: "last_modified",
        width: 200,
      },{
        title: 'Filename',
        dataIndex: "filename",
        render:  (text, row) => {
            if(row['permission'][0] === "d") {
                return <Button type="link" onClick={()=> gotoDir(dir + "/"+ row['filename'])}> {row['filename']}</Button>
            }
            var fileLink = API_ENDPOINT + '/vms/' + deviceName + '/files?path=' + encodeURIComponent(dir + "/"+ row['filename']);
            return <Button type="link" style={{color: 'black'}} href={fileLink}> {row['filename']}</Button>
        }
      },
  ];

  function humanFileSize(bytes, si=false, dp=1) {
    const thresh = si ? 1000 : 1024;
    if (Math.abs(bytes) < thresh) {
      return bytes + ' B';
    }
    const units = si 
      ? ['kB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'] 
      : ['KiB', 'MiB', 'GiB', 'TiB', 'PiB', 'EiB', 'ZiB', 'YiB'];
    let u = -1;
    const r = 10**dp;
    do {
      bytes /= thresh;
      ++u;
    } while (Math.round(Math.abs(bytes) * r) / r >= thresh && u < units.length - 1);
    return bytes.toFixed(dp) + ' ' + units[u];
  }  

  const getFiles = useCallback((path) => {
    var url = API_ENDPOINT + '/vms/' + deviceName + '/dir'
    axios.get(url, { params: { "path": dir } })
    .then(function (response) {
        var records = response.data.files;
        var files = [];
        for (let i=0; i < records.length; i++) {
            var items = records[i].split("|");
            var last_modified = new Date(parseInt(items[4].split('.')[0])*1000)
            files.push({
                key: i,
                permission: items[0],
                user: items[1],
                group: items[2],
                size: humanFileSize(items[3], false, 2),
                last_modified: last_modified.toLocaleString('en-US', { timeZone: 'Asia/Singapore' }),
                filename: i===0 ? ".." : items[5]
            })
        }
        files.sort(function(a,b) {
            return a.filename-b.filename
        });
        setTableData(files);
    })
    .catch(function (error) {
        console.log(error);
    })
  }, [dir, API_ENDPOINT, deviceName]);

  useEffect(() => {
    getFiles();
  }, [getFiles])

  return (
    <div className="detail-tab-content">
      <Input value={dir} prefix={<FolderOpenOutlined />} disabled={true}/>
      <Table 
        className="file-explorer"
        columns={columns} 
        dataSource={tableData} 
        pagination={false} 
        scroll={{ y: "60vh" }}
      //   loading={props.isLoading}
      />
    </div>
  )
}

export default FileExplorer;