import { useRef, useEffect,useMemo, useCallback } from 'react';
import { XTerm } from 'xterm-for-react'
import { FitAddon } from 'xterm-addon-fit';
import { AttachAddon } from 'xterm-addon-attach';

function WebTerminal(props){
    const xtermRef = useRef(null);
    const fitAddon = useMemo(() => new FitAddon(),[]);
    const WS_ENDPOINT = "ws://"+  window.location.hostname + ":" + process.env.REACT_APP_API_PORT + "/api/v1";
    // TODO investigate why attachAddOn will
    const ws = useMemo(() => new WebSocket(WS_ENDPOINT + "/vms/" + props.deviceName+ "/ws"), [WS_ENDPOINT, props.deviceName]);
    const attachAddon = useMemo(() => new AttachAddon(ws),[ws]);
    
    useEffect(() => {
        if(!props.isHidden){
            fitAddon.fit();
        }
        // call any method in XTerm.js by using 'xtermRef.current.terminal.[Whatever you want to call]
    })

    const resizeCallback = useCallback(() => {
        if (!props.isHidden) {
            fitAddon.fit()
        }
    }, [fitAddon, props.isHidden]);

    useEffect(() => {
        window.addEventListener('resize', resizeCallback);
        return () => {
            window.removeEventListener('resize', resizeCallback);
        }
    }, [props.isHidden, resizeCallback])
    
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