import RFB from '@novnc/novnc/core/rfb';
import { useEffect, useRef} from 'react';
import { message } from 'antd';

function VNCDisplay(props){
    const rfb = useRef(null);
    const player = useRef(null);

    // successfully connected to a server
    function connectedToServer(e) {
        console.log("VNC connected");
    }

    // This function is called when we are disconnected
    function disconnectedFromServer(e) {
        if (e.detail.clean) {
            message.warning("Disconnected from the device's VNC display. Please try refresh the page.", 0)
        } else {
            message.warning("Unable to connect to the device's VNC display. Please try refresh the page.", 0);
        }
    }

    useEffect(() => {
        // Creating a new RFB object will start a new connection
        rfb.current = new RFB(player.current, props.url, {
            wsProtocols: ['binary', 'base64'],
        });
        console.log("new RFB conn")
        // Add listeners to important events from the RFB module
        rfb.current.addEventListener("connect",  connectedToServer);
        rfb.current.addEventListener("disconnect", disconnectedFromServer);

        // Set parameters that can be changed on an active connection
        rfb.current.viewOnly = false;
        rfb.current.scaleViewport = true;
        rfb.current.showDotCursor = true;
    }, [props.url]);

    return (
        <div id="vnc-canvas" ref={player}></div>
    )
}

export default VNCDisplay