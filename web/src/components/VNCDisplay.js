import RFB from '@novnc/novnc/core/rfb';
import { useEffect, useRef} from 'react';

function VNCDisplay(props){
    const rfb = useRef(null);
    // When this function is called we have
    // successfully connected to a server
    function connectedToServer(e) {
        console.log("VNC connected");
    }

    // This function is called when we are disconnected
    function disconnectedFromServer(e) {
        if (e.detail.clean) {
            console.log("Disconnected");
        } else {
            console.log("Something went wrong, connection is closed");
        }
    }

    useEffect(() => {
        // Creating a new RFB object will start a new connection
        rfb.current = new RFB(document.getElementById('vnc-canvas'), props.url, {
            wsProtocols: ['binary', 'base64'],
        });

        // Add listeners to important events from the RFB module
        rfb.current.addEventListener("connect",  connectedToServer);
        rfb.current.addEventListener("disconnect", disconnectedFromServer);

        // Set parameters that can be changed on an active connection
        rfb.current.viewOnly = false;
        rfb.current.scaleViewport = true;
        rfb.current.showDotCursor = true;
    }, [props.url]);

    return (
        <div id="vnc-canvas"></div>
    )
}

export default VNCDisplay