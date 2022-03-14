import { useRef, useEffect,useMemo, useCallback } from 'react';
import { XTerm } from 'xterm-for-react'
import { FitAddon } from 'xterm-addon-fit';
import { AttachAddon } from 'xterm-addon-attach';

function WebTerminal(props){
    //Call any method in XTerm.js by using 'xtermRef.current.terminal.abc
    const xtermRef = useRef(null);
    const fitAddon = useMemo(() => new FitAddon(),[]);
    const WS_ENDPOINT = "ws://"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1";

    // Calculate terminal column and line size and send it to the backend to adjust the tty size
    const sendTerminalSize = useCallback((xtermCore, wsConn) => {
        var width = xtermCore._renderService.dimensions.canvasWidth;
        var height = xtermCore._renderService.dimensions.canvasHeight;
        var charWidth = xtermCore._renderService.dimensions.actualCellWidth;
        var charHeight = xtermCore._renderService.dimensions.actualCellHeight;
        var cols = Math.floor(width/charWidth);
        var lines = Math.floor(height/charHeight);
        wsConn.send("$$MATRISEA_RESIZE " + cols +" " + lines + "");
    }, []);

    // Terminal's websocket connection
    const ws = useMemo(() => {
        console.log("new terminal conn"); 
        let newWS = new WebSocket(WS_ENDPOINT + "/vms/" + props.deviceName+ "/ws");
        newWS.onopen = () => {
            if (xtermRef.current && ws && ws.readyState === 1){
                sendTerminalSize(xtermRef.current.terminal._core, ws)
            }
        };
        return newWS;
    }, [WS_ENDPOINT, props.deviceName, sendTerminalSize]);
    const attachAddon = useMemo(() => new AttachAddon(ws),[ws]);

    const resizeCallback = useCallback(() => {
        if (!props.isHidden) {
            fitAddon.fit()
            if (xtermRef.current && ws && ws.readyState === 1){
                sendTerminalSize(xtermRef.current.terminal._core, ws)
            }
        }
    }, [fitAddon, props.isHidden, ws, sendTerminalSize]);

    useEffect(() => {
        if (!props.isHidden) {
            fitAddon.fit()
        }
        // Update the backend with the latest terminal window size 
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