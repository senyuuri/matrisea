import { useRef, useEffect } from 'react';
import { XTerm } from 'xterm-for-react'
import { FitAddon } from 'xterm-addon-fit';
import { AttachAddon } from 'xterm-addon-attach';

function WebTerminal(){
    const WS_ENDPOINT = process.env.REACT_APP_WS_ENDPOINT
    const ws = new WebSocket(`${WS_ENDPOINT}/vms/matrisea-cvd-BCmGbk/ws`);

    const xtermRef = useRef(null);
    const fitAddon = new FitAddon();
    const attachAddon = new AttachAddon(ws);
    
    useEffect(() => {
        // call any method in XTerm.js by using 'xterm xtermRef.current.terminal.[What you want to call]
        fitAddon.fit();
    }, [fitAddon])
    return (
        <XTerm 
            ref={xtermRef}
            addons={[fitAddon, attachAddon]}
        />
    );
}
export default WebTerminal;