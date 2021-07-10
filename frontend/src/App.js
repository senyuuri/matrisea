import React, { useState, useCallback, useEffect } from 'react';
import { BrowserRouter as Router, Route, Switch } from 'react-router-dom'
import { Layout, Breadcrumb, Row, Button, } from 'antd';

import DeviceList from "./DeviceList"
import DeviceDetail from './DeviceDetail';

import './App.css';

const { Header, Content } = Layout;

function App() {
  useEffect(() => {
    document.title = "Matrisea"
  }, []);

  return (<Layout className="layout" style={{ minHeight: "100vh" }}>
    <Header>
      <div className="logo">
        <img src="/logo512.png" style={{maxWidth: "100%", maxHeight: "100%" }}></img>
      </div>
      {/* <h1 className="logo-text"> Matrisea</h1> */}
    </Header>
    <Content style={{ padding: '0 50px' }}>
      <Router>
        <Switch>
          <Route exact path={"/"} component={DeviceList} />
          <Route exact path={"/device/:device_name"} component={DeviceDetail} />
        </Switch>
      </Router>
    </Content>
  </Layout>)
}

export default App;
