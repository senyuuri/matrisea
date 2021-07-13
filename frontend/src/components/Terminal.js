import { useRef, useEffect } from 'react';
import { XTerm } from 'xterm-for-react'
import { FitAddon } from 'xterm-addon-fit';
import { AttachAddon } from 'xterm-addon-attach';

function WebTerminal(){
    // TODO check environment and change to container host name in prod
    const ws = new WebSocket("ws://127.0.0.1:8080/api/v1/vms/matrisea-cvd-BCmGbk/ws");

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