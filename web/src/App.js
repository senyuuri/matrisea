import React, {useEffect} from 'react';
import { BrowserRouter as Router, Route, Switch, useLocation } from 'react-router-dom'
import { Layout } from 'antd';
import { WsContext } from './Context';

import DeviceList from "./DeviceList"
import DeviceDetail from './DeviceDetail';

import './App.css';

const { Header, Content } = Layout;
const WS_ENDPOINT = "ws://"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1/ws"

function App() {
  const location = useLocation()
  const [ws, setWs] = React.useState();
  
  useEffect(() => {
    const ws = new WebSocket(WS_ENDPOINT);
    ws.onopen = () => {
      console.log("ws opened");
    };
    ws.onclose = (e) => console.log("ws closed", e);
    ws.onerror = (e) => console.log("ws error", e);
    setWs(ws);
  }, []);

  return (
  <Layout className="layout" style={{ minHeight: "100vh" }}>
    <Header>
      <div className="logo">
        <img alt="logo" src="/logo512.png" style={{maxWidth: "100%", maxHeight: "100%" }}></img>
      </div>
      {/* <h1 className="logo-text"> Matrisea</h1> */}
    </Header>
    <WsContext.Provider value={ws}>
      <Content style={{ padding: '0 50px' }}>
        <Switch location={location}>
          <React.Fragment>
            <Route location={location} exact path={"/"} component={DeviceList} key="router-list"/>
            <Route location={location} exact path={"/device/:device_name"} component={DeviceDetail} key="router-detail"/>
          </React.Fragment>
        </Switch>
      </Content>
    </WsContext.Provider>
  </Layout>)
}

export default App;
