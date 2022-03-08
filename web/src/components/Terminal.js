import { useRef, useEffect,useMemo, useCallback } from 'react';
import { XTerm } from 'xterm-for-react'
import { FitAddon } from 'xterm-addon-fit';
import { AttachAddon } from 'xterm-addon-attach';

function WebTerminal(props){
    //call any method in XTerm.js by using 'xtermRef.current.terminal.abc
    const xtermRef = useRef(null);
    const fitAddon = useMemo(() => new FitAddon(),[]);
    const WS_ENDPOINT = "ws://"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1";
    const ws = useMemo(() => {
        console.log("new terminal conn"); 
        let newWS = new WebSocket(WS_ENDPOINT + "/vms/" + props.deviceName+ "/ws");
        newWS.onopen = () => {
            if (xtermRef.current && ws && ws.readyState === 1){
                var width = xtermRef.current.terminalRef.current.offsetWidth;
                var height = xtermRef.current.terminalRef.current.offsetHeight;
                ws.send("$$MATRISEA_RESIZE " + width +" " + height + "");
            }
        };
        return newWS;
    }, [WS_ENDPOINT, props.deviceName]);
    const attachAddon = useMemo(() => new AttachAddon(ws),[ws]);

    const resizeCallback = useCallback(() => {
        if (!props.isHidden) {
            if (xtermRef.current && ws && ws.readyState === 1){
                var width = xtermRef.current.terminalRef.current.offsetWidth;
                var height = xtermRef.current.terminalRef.current.offsetHeight;
                ws.send("$$MATRISEA_RESIZE " + width +" " + height + "");
            }
            fitAddon.fit()
        }
    }, [fitAddon, props.isHidden, ws]);

    useEffect(() => {
        if (!props.isHidden) {
            fitAddon.fit()
        }
        window.addEventListener('resize', resizeCallback);
        return () => {
            window.removeEventListener('resize', resizeCallback);
        }
    }, [props.isHidden, resizeCallback, ws, fitAddon])
    
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