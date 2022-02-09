import { useRef, useEffect,useMemo } from 'react';
import { XTerm } from 'xterm-for-react'
import { FitAddon } from 'xterm-addon-fit';
import { AttachAddon } from 'xterm-addon-attach';

function WebTerminal(props){
    const WS_ENDPOINT = "ws://"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1";
    const ws = useMemo(() => new WebSocket(WS_ENDPOINT + "/vms/matrisea-cvd-" +props.deviceName+ "/ws"), [WS_ENDPOINT, props.deviceName]);

    const xtermRef = useRef(null);
    const fitAddon = useMemo(() => new FitAddon(),[]);
    const attachAddon = useMemo(() => new AttachAddon(ws),[ws]);
    
    useEffect(() => {
        fitAddon.fit();
        // call any method in XTerm.js by using 'xtermRef.current.terminal.[Whatever you want to call]
    }, [fitAddon])

    useEffect(() => {
        window.addEventListener('resize', () => {
            fitAddon.fit()
        })
    })
    
    const opts = {
        screenKeys: true,
        cursorBlink: false,
        scrollback: 9999999, // unlimited
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