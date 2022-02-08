import { useRef, useEffect,useMemo } from 'react';
import { XTerm } from 'xterm-for-react'
import { FitAddon } from 'xterm-addon-fit';
import { AttachAddon } from 'xterm-addon-attach';

function WebTerminal(props){
    const WS_ENDPOINT = "ws://"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1"
    const ws = new WebSocket(WS_ENDPOINT + "/vms/matrisea-cvd-" +props.deviceName+ "/ws");

    const xtermRef = useRef(null);
    const fitAddon = useMemo(() => new FitAddon(),[]);
    const attachAddon = new AttachAddon(ws);
    
    useEffect(() => {
        fitAddon.fit();
        // call any method in XTerm.js by using 'xterm xtermRef.current.terminal.[What you want to call]
    }, [fitAddon])

    useEffect(() => {
        window.addEventListener('resize', () => {
            fitAddon.fit()
        })
    })
    
    const opts = {
        screenKeys: true,
        cursorBlink: false
    };

    return (
        <XTerm 
            options={opts}
            ref={xtermRef}
            addons={[fitAddon, attachAddon]}
        />
    );
}
export default WebTerminal;