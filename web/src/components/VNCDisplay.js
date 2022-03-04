import RFB from '@novnc/novnc/core/rfb';
import { useCallback, useEffect, useRef} from 'react';
import { message } from 'antd';

function VNCDisplay(props){
    const rfb = useRef(null);
    const player = useRef(null);

    // successfully connected to a server
    function connectedToServer(e) {
        console.log("VNC connected");
    }

    const connectVNC = useCallback(() => {
        console.log("Initiating VNC connection")
        // Creating a new RFB object will start a new connection
        rfb.current = new RFB(player.current, props.url, {
            wsProtocols: ['binary', 'base64'],
        });
        // Add listeners to important events from the RFB module
        rfb.current.addEventListener("connect",  connectedToServer);
        rfb.current.addEventListener("disconnect", (e) => {
            console.log('VNC disconnected. clean exit?', e.detail.clean);
            if (!e.detail.clean) {
                console.log('Unexpected VNC disconnect, reconnecting in 10s')
                setTimeout(function() {
                    message.warning("Unable to connect to the device's display. Retry after 10s", 10);
                    connectVNC();
                }, 10000); 
            }
        });
        
        // Set parameters that can be changed on an active connection
        rfb.current.viewOnly = false;
        rfb.current.scaleViewport = true;
        rfb.current.showDotCursor = true;
    },[props.url])

    useEffect(() => {
        connectVNC()
        return () => {
            console.log("Clean up VNC object")
            rfb.current = null;
        }
    }, [connectVNC]);

    return (
        <div id="vnc-canvas" ref={player}></div>
    )
}

export default VNCDisplay