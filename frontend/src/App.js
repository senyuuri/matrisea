import React from 'react';
import { BrowserRouter as Router, Route, Switch, useLocation } from 'react-router-dom'
import { Layout } from 'antd';
import QueueAnim from 'rc-queue-anim';

import DeviceList from "./DeviceList"
import DeviceDetail from './DeviceDetail';

import './App.css';

const { Header, Content } = Layout;

function App() {
  const location = useLocation()

  return (<Layout className="layout" style={{ minHeight: "100vh" }}>
    <Header>
      <div className="logo">
        <img alt="logo" src="/logo512.png" style={{maxWidth: "100%", maxHeight: "100%" }}></img>
      </div>
      {/* <h1 className="logo-text"> Matrisea</h1> */}
    </Header>
    <Content style={{ padding: '0 50px' }}>
      <Switch location={location}>
        <React.Fragment>
          <QueueAnim type={['right', 'left']} className="router-wrap">
            <Route location={location} exact path={"/"} component={DeviceList} key="router-list"/>
            <Route location={location} exact path={"/device/:device_name"} component={DeviceDetail} key="router-detail"/>
          </QueueAnim>
        </React.Fragment>
      </Switch>
    </Content>
  </Layout>)
}

export default App;
